// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package ceph

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
)

func createKeyFile(imageName string, key string) (string, func() error, error) {
	noOpCleanup := func() error { return nil }
	file, err := os.CreateTemp("", fmt.Sprintf("%s-*", imageName))
	if err != nil {
		return "", noOpCleanup, fmt.Errorf("failed to create temp file: %w", err)
	}
	cleanup := func() error {
		return os.Remove(file.Name())
	}

	_, err = file.WriteString(key)
	if err != nil {
		return "", cleanup, fmt.Errorf("failed to write key to temp file: %w", err)
	}

	return file.Name(), cleanup, nil
}

func connectToRados(ctx context.Context, monitors, user, keyfile string) (*rados.Conn, error) {
	args := []string{"-m", monitors, "--keyfile=" + keyfile}
	conn, err := rados.NewConnWithUser(user)
	if err != nil {
		return nil, fmt.Errorf("creating a new connection failed: %w", err)
	}
	err = conn.ParseCmdLineArgs(args)
	if err != nil {
		return nil, fmt.Errorf("parsing cmdline args (%v) failed: %w", args, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- conn.Connect()
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("ceph connect timeout. monitors: %s, user: %s: %w", monitors, user, ctx.Err())
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("connecting failed: %w", err)
		}
	}

	return conn, nil
}

func (p *plugin) GetSize(ctx context.Context, spec *api.VolumeSpec) (int64, error) {
	log := logr.FromContextOrDiscard(ctx)

	if spec.Connection == nil {
		return 0, errors.New("connection data is not set")
	}

	userID, userKey, err := readSecretData(spec.Connection.SecretData)
	if err != nil {
		return 0, fmt.Errorf("error reading secret data: %w", err)
	}

	monitors, ok := spec.Connection.Attributes[volumeAttributesMonitorsKey]
	if !ok {
		return 0, fmt.Errorf("no monitors at %s", volumeAttributesMonitorsKey)
	}

	parts := strings.SplitN(spec.Connection.Handle, "/", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("handle is not well formated: expected 'pool/image' format but got %s", spec.Connection.Handle)
	}
	poolName, imageName := parts[0], parts[1]

	keyFile, cleanup, err := createKeyFile(imageName, userKey)
	defer func() {
		if err := cleanup(); err != nil {
			log.Error(err, "failed to cleanup key file")
		}
	}()
	if err != nil {
		return 0, fmt.Errorf("failed to create temp key file: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	conn, err := connectToRados(timeoutCtx, monitors, userID, keyFile)
	if err != nil {
		return 0, fmt.Errorf("failed to open connection: %w", err)
	}

	ioCtx, err := conn.OpenIOContext(poolName)
	if err != nil {
		return 0, fmt.Errorf("failed to open io context: %w", err)
	}

	image, err := rbd.OpenImage(ioCtx, imageName, rbd.NoSnapshot)
	if err != nil {
		return 0, fmt.Errorf("failed to open image: %w", err)
	}

	size, err := image.GetSize()
	if err != nil {
		if closeErr := image.Close(); closeErr != nil {
			return 0, errors.Join(err, fmt.Errorf("unable to close image: %w", closeErr))
		}
		return 0, fmt.Errorf("failed to set encryption format: %w", err)
	}

	if err := image.Close(); err != nil {
		return 0, fmt.Errorf("failed to close rbd image: %w", err)
	}

	return int64(size), nil
}
