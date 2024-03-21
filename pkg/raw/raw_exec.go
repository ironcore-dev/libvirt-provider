// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package raw

import (
	"fmt"
	"io"
	"os"

	"github.com/go-logr/logr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Exec struct{}

const filePerm = 0660

func (Exec) Create(filename string, opts ...CreateOption) error {
	o := &CreateOptions{}
	o.ApplyOptions(opts)
	log := ctrl.Log.WithName("raw-disk")

	if o.SourceFile == "" {
		if o.Size == nil {
			return fmt.Errorf("must specify Size when creating without source file")
		}
		seek := *o.Size
		// Position the file cursor one byte before the desired seek position to write a single byte,
		// to ensure that data is written at the exact byte position specified by seek.
		err := createEmptyFileWithSeek(log, filename, seek-1)
		if err != nil {
			return fmt.Errorf("failed creating the empty ephemeral disk at %s: %w", filename, err)
		}
	} else {
		err := copyFile(log, o.SourceFile, filename)
		if err != nil {
			return fmt.Errorf("failed creating virtual disk image, source: %s, destination: %s: %w", o.SourceFile, filename, err)
		}
	}

	return nil
}

func createEmptyFileWithSeek(log logr.Logger, filename string, seek int64) error {
	dstFile, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePerm)
	if err != nil {
		return fmt.Errorf("failed opening destination file: %w", err)
	}

	defer func() {
		destErr := dstFile.Close()
		if destErr != nil {
			log.WithName("createEmptyFileWithSeek").Error(destErr, "error closing file", "filename", filename)
		}
	}()

	_, err = dstFile.Seek(seek, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed seeking destination file: %w", err)
	}

	_, err = dstFile.Write([]byte{0})

	return err
}

func copyFile(log logr.Logger, src, dst string) error {
	log = log.WithName("copyFile").WithValues("source", src, "destination", dst)
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed opening source file: %w", err)
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			log.Error(err, "error closing source file")
		}
	}()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePerm)
	if err != nil {
		return fmt.Errorf("failed opening destination file: %w", err)
	}

	defer func() {
		if err := dstFile.Close(); err != nil {
			log.Error(err, "error closing destination file")
		}
	}()

	_, err = io.Copy(dstFile, srcFile)

	return err
}

func init() {
	utilruntime.Must(impls.Add("exec", 0, Exec{}))
}
