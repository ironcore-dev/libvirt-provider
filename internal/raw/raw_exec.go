// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
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

// Create writes a raw disk image at filename.
// A source file, if given, is copied and then extended to the requested size.
// It returns an error if the requested size is smaller than the source.
func (Exec) Create(filename string, opts ...CreateOption) (err error) {
	o := &CreateOptions{}
	o.ApplyOptions(opts)
	log := ctrl.Log.WithName("raw-disk").WithValues("filename", filename)

	defer func() {
		if err != nil {
			os.Remove(filename)
		}
	}()

	if o.Size != nil && *o.Size <= 0 {
		return fmt.Errorf("size must be greater than zero, got %d", *o.Size)
	}

	if o.SourceFile == "" {
		if o.Size == nil {
			return fmt.Errorf("must specify Size when creating without source file")
		}
		seek := *o.Size
		// Position the file cursor one byte before the desired seek position to write a single byte,
		// to ensure that data is written at the exact byte position specified by seek.
		if err := createEmptyFileWithSeek(log, filename, seek-1); err != nil {
			return fmt.Errorf("failed creating the empty ephemeral disk at %s: %w", filename, err)
		}
	} else {
		var wantSize int64
		if o.Size != nil {
			wantSize = *o.Size
		}

		if wantSize > 0 {
			fi, err := os.Stat(o.SourceFile)
			if err != nil {
				return fmt.Errorf("could not stat %q: %w", o.SourceFile, err)
			}
			if fi.Size() > wantSize {
				return fmt.Errorf("cannot create %q at %d: source file %q is already %d", filename, wantSize, o.SourceFile, fi.Size())
			}
		}

		if err := copyFile(log, o.SourceFile, filename); err != nil {
			return fmt.Errorf("failed creating virtual disk image, source: %s, destination: %s: %w", o.SourceFile, filename, err)
		}

		if wantSize > 0 {
			if err := os.Truncate(filename, wantSize); err != nil {
				return fmt.Errorf("resizing file: %w", err)
			}
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
		if err := dstFile.Close(); err != nil {
			log.Error(err, "error closing file in createEmptyFileWithSeek")
		}
	}()

	if _, err = dstFile.Seek(seek, io.SeekStart); err != nil {
		return fmt.Errorf("failed seeking destination file: %w", err)
	}

	if _, err = dstFile.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to write data to destination file: %w", err)
	}

	return nil
}

func copyFile(log logr.Logger, src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed opening source file: %w", err)
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			log.Error(err, "error closing source file in copyFile", "path", src)
		}
	}()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePerm)
	if err != nil {
		return fmt.Errorf("failed opening destination file: %w", err)
	}

	defer func() {
		if err := dstFile.Close(); err != nil {
			log.Error(err, "error closing destination file in copyFile")
		}
	}()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy data from source file to destination file: %w", err)
	}

	return nil
}

func init() {
	utilruntime.Must(impls.Add("exec", 0, Exec{}))
}
