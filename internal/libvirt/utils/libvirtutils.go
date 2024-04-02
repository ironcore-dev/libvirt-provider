// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket"
	"github.com/digitalocean/go-libvirt/socket/dialers"
	"github.com/google/uuid"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"libvirt.org/go/libvirtxml"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	defaultSocket = "/var/run/libvirt/libvirt-sock"
)

var (
	log = ctrl.Log.WithName("libvirtutils")
)

func wellKnownSocketPaths() []string {
	paths := []string{defaultSocket}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		paths = append(paths, filepath.Join(homeDir, ".cache", "libvirt", "libvirt-sock"))
	}

	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		paths = append(paths, filepath.Join("/", "opt", "homebrew", "var", "run", "libvirt", "libvirt-sock"))
	}

	return paths
}

func GetDialer(socket, address string) (socket.Dialer, error) {
	if socket != "" {
		log.V(1).Info("Using explicit local socket", "Socket", socket)
		return dialers.NewLocal(dialers.WithSocket(socket), dialers.WithLocalTimeout(1*time.Second)), nil
	}
	if address != "" {
		log.V(1).Info("Using explicit remote socket", "Address", address)
		return dialers.NewRemote(address), nil
	}

	wellKnownSocketPaths := wellKnownSocketPaths()
	log.V(1).Info("Probing well known socket paths", "WellKnownSocketPaths", wellKnownSocketPaths)
	for _, wellKnownSocketPath := range wellKnownSocketPaths {
		stat, err := os.Stat(wellKnownSocketPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Error(err, "Error checking socket path", "SocketPath", wellKnownSocketPath)
		} else if err == nil {
			if (stat.Mode() & os.ModeSocket) != 0 {
				log.V(1).Info("Determined socket", "Socket", wellKnownSocketPath)
				return dialers.NewLocal(dialers.WithSocket(wellKnownSocketPath)), nil
			}
		}
	}
	return nil, fmt.Errorf("could not determine libvirt dialer to use")
}

func wellKnownConnectURIs() []libvirt.ConnectURI {
	var uris []libvirt.ConnectURI
	if defaultURI := os.Getenv("LIBVIRT_DEFAULT_URI"); defaultURI != "" {
		uris = append(uris, libvirt.ConnectURI(defaultURI))
	}

	uris = append(uris, libvirt.QEMUSystem)
	return uris
}

var (
	expectedConnectErrorMessageRegex = regexp.MustCompile(`\Qinternal error: unexpected qemu URI path\E|\Qno polkit agent available\E`)
)

func Connect(lv *libvirt.Libvirt, uri string) error {
	if uri != "" {
		log.V(1).Info("Connecting to explicit uri", "URI", uri)
		return lv.ConnectToURI(libvirt.ConnectURI(uri))
	}

	wellKnownConnectURIs := wellKnownConnectURIs()
	log.V(1).Info("Probing well known connect URIs", "WellKnownConnectURIs", wellKnownConnectURIs)
	for _, wellKnownConnectURI := range wellKnownConnectURIs {
		if err := lv.ConnectToURI(wellKnownConnectURI); err != nil {
			var lvErr libvirt.Error
			if !errors.As(err, &lvErr) {
				return err
			}

			if !expectedConnectErrorMessageRegex.MatchString(lvErr.Message) {
				return err
			}
			continue
		}
		log.V(1).Info("Determined connect uri", "URI", wellKnownConnectURI)
		return nil
	}
	return fmt.Errorf("could not determine connect uri")
}

func GetLibvirt(socket, address, uri string) (*libvirt.Libvirt, error) {
	dialer, err := GetDialer(socket, address)
	if err != nil {
		return nil, err
	}

	lv := libvirt.NewWithDialer(dialer)
	if err := Connect(lv, uri); err != nil {
		return nil, err
	}
	return lv, nil
}

func IsErrorCode(err error, codes ...libvirt.ErrorNumber) bool {
	var lErr libvirt.Error
	if !errors.As(err, &lErr) {
		return false
	}

	for _, code := range codes {
		if lErr.Code == uint32(code) {
			return true
		}
	}
	return false
}

func IgnoreErrorCode(err error, codes ...libvirt.ErrorNumber) error {
	if IsErrorCode(err, codes...) {
		return nil
	}
	return err
}

func UUIDStringToBytes(uid string) libvirt.UUID {
	u := uuid.MustParse(uid)
	data, err := u.MarshalBinary()
	utilruntime.Must(err)
	var lUUID libvirt.UUID
	copy(lUUID[:], data)
	return lUUID
}

func ApplySecret(lv *libvirt.Libvirt, secret *libvirtxml.Secret, value []byte) error {
	data, err := secret.Marshal()
	if err != nil {
		return err
	}

	if _, err := lv.SecretLookupByUUID(UUIDStringToBytes(secret.UUID)); err != nil {
		if !IsErrorCode(err, libvirt.ErrNoSecret) {
			return fmt.Errorf("error looking up secret %s: %w", secret.UUID, err)
		}

		if _, err := lv.SecretDefineXML(data, 0); err != nil {
			return fmt.Errorf("error defining secret %s: %w", secret.UUID, err)
		}
	}

	if err := lv.SecretSetValue(libvirt.Secret{
		UUID: UUIDStringToBytes(secret.UUID),
	}, value, 0); err != nil {
		return fmt.Errorf("error setting secret value: %w", err)
	}

	return nil
}
