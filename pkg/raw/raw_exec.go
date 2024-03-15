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

const filePerm = 0644

func (Exec) Create(filename string, opts ...CreateOption) error {
	o := &CreateOptions{}
	o.ApplyOptions(opts)
	log := ctrl.Log

	if o.SourceFile == "" {
		if o.Size == nil {
			return fmt.Errorf("must specify Size when creating without source file")
		}
		seek := *o.Size
		// Position the file cursor one byte before the desired seek position to write a single byte,
		// to ensure that data is written at the exact byte position specified by seek.
		err := createEmptyFileWithSeek(log, filename, seek-1)
		if err != nil {
			return fmt.Errorf("failed creating the empty ephemeral disk: %w", err)
		}
	} else {
		err := copyFile(log, o.SourceFile, filename)
		if err != nil {
			return fmt.Errorf("failed creating the image for virtual disk: %w", err)
		}
	}

	return nil
}

func createEmptyFileWithSeek(log logr.Logger, filename string, seek int64) error {
	log = log.WithName("createEmptyFileWithSeek")
	dstFile, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePerm)
	if err != nil {
		return err
	}

	defer func() {
		destErr := dstFile.Close()
		if destErr != nil {
			log.Error(destErr, "error closing file", "Path", dstFile)
		}
	}()

	_, err = dstFile.Seek(seek, io.SeekStart)
	if err != nil {
		return err
	}

	_, err = dstFile.Write([]byte{0})
	if err != nil {
		return err
	}

	return nil
}

func copyFile(log logr.Logger, src, dst string) error {
	log = log.WithName("copyFile")
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		srcErr := srcFile.Close()
		if srcErr != nil {
			log.Error(srcErr, "error closing file", "Path", srcFile)
		}
	}()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePerm)
	if err != nil {
		return err
	}

	defer func() {
		destErr := dstFile.Close()
		if destErr != nil {
			log.Error(destErr, "error closing file", "Path", dstFile)
		}
	}()

	_, err = io.Copy(dstFile, srcFile)

	return err
}

func init() {
	utilruntime.Must(impls.Add("exec", 0, Exec{}))
}
