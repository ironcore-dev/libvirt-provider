// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package raw_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ironcore-dev/libvirt-provider/internal/raw"
)

const (
	sourceSize     int64 = 4096
	grownSize      int64 = sourceSize * 4
	emptyDiskSize  int64 = sourceSize * 2
	undersizedSize int64 = sourceSize / 2
)

func TestExecCreateDiskSize(t *testing.T) {
	tests := []struct {
		name       string
		opts       func(sourceFile string) []raw.CreateOption
		expectSize int64
	}{
		{
			name: "grows the disk to the requested size",
			opts: func(sourceFile string) []raw.CreateOption {
				return []raw.CreateOption{raw.WithSourceFile(sourceFile), raw.WithSize(grownSize)}
			},
			expectSize: grownSize,
		},
		{
			name: "keeps the source size when no size is requested",
			opts: func(sourceFile string) []raw.CreateOption {
				return []raw.CreateOption{raw.WithSourceFile(sourceFile)}
			},
			expectSize: sourceSize,
		},
		{
			name: "creates an empty disk of the requested size without a source",
			opts: func(string) []raw.CreateOption {
				return []raw.CreateOption{raw.WithSize(emptyDiskSize)}
			},
			expectSize: emptyDiskSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			diskFile := filepath.Join(dir, "disk.raw")

			if err := (raw.Exec{}).Create(diskFile, tt.opts(writeSourceFile(t, dir))...); err != nil {
				t.Fatalf("Create() returned %v, want no error", err)
			}

			if got := fileSize(t, diskFile); got != tt.expectSize {
				t.Errorf("disk size = %d, want %d", got, tt.expectSize)
			}
		})
	}
}

func TestExecCreateRejectsSizeSmallerThanSource(t *testing.T) {
	dir := t.TempDir()
	diskFile := filepath.Join(dir, "disk.raw")

	err := (raw.Exec{}).Create(diskFile, raw.WithSourceFile(writeSourceFile(t, dir)), raw.WithSize(undersizedSize))
	if err == nil {
		t.Fatal("Create() should return an error")
	}

	// localdisk.Apply skips creation when the disk already exists.
	_, statErr := os.Stat(diskFile)
	if statErr == nil {
		t.Error("disk file exists after a rejected create, expected absent")
		return
	}
	if !os.IsNotExist(statErr) {
		t.Fatalf("stat %s: %v", diskFile, statErr)
	}
}

func TestExecCreateRequiresSizeWithoutSource(t *testing.T) {
	diskFile := filepath.Join(t.TempDir(), "disk.raw")

	if err := (raw.Exec{}).Create(diskFile); err == nil {
		t.Fatal("Create() should return an error")
	}
}

func writeSourceFile(t *testing.T, dir string) string {
	t.Helper()

	sourceFile := filepath.Join(dir, "source.raw")
	if err := os.WriteFile(sourceFile, make([]byte, sourceSize), 0o600); err != nil {
		t.Fatalf("writing source file: %v", err)
	}
	return sourceFile
}

func fileSize(t *testing.T, filename string) int64 {
	t.Helper()

	stat, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat %s: %v", filename, err)
	}
	return stat.Size()
}
