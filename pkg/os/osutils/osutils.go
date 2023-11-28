// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package osutils

import (
	"errors"
	"fmt"
	"os"
)

func checkStatExists(filename string, check func(stat os.FileInfo) error) (bool, error) {
	stat, err := os.Stat(filename)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		return false, nil
	}
	if err := check(stat); err != nil {
		return false, err
	}
	return true, nil
}

func RegularFileExists(filename string) (bool, error) {
	return checkStatExists(filename, func(stat os.FileInfo) error {
		if !stat.Mode().IsRegular() {
			return fmt.Errorf("no regular file at %s", filename)
		}
		return nil
	})
}

func DirExists(filename string) (bool, error) {
	return checkStatExists(filename, func(stat os.FileInfo) error {
		if !stat.Mode().IsRegular() {
			return fmt.Errorf("no directory at %s", filename)
		}
		return nil
	})
}

// WriteFileIfNoFileExists checks if a file exists via RegularFileExists (returning an error if a file exists
// but not being a regular file). If it exists, it returns, otherwise, it writes the given content.
func WriteFileIfNoFileExists(name string, data []byte, perm os.FileMode) error {
	if ok, err := RegularFileExists(name); err != nil || ok {
		return err
	}
	return os.WriteFile(name, data, perm)
}
