// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package raw_test

import (
	"os"
	"path/filepath"

	"github.com/ironcore-dev/libvirt-provider/internal/raw"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	sourceSize     int64 = 4096
	grownSize      int64 = sourceSize * 4
	emptyDiskSize  int64 = sourceSize * 2
	undersizedSize int64 = sourceSize / 2
)

var _ = Describe("Exec", func() {
	Describe("Create", func() {
		var dir, diskFile string

		BeforeEach(func() {
			dir = GinkgoT().TempDir()
			diskFile = filepath.Join(dir, "disk.raw")
		})

		When("source file configured", func() {
			var imageSource string

			BeforeEach(func() {
				imageSource = writeSourceFile(dir)
			})

			DescribeTable("sizes a new disk", func(expectSize int64, sizeOpts ...raw.CreateOption) {
				e := raw.Exec{}
				opts := append([]raw.CreateOption{raw.WithSourceFile(imageSource)}, sizeOpts...)

				Expect(e.Create(diskFile, opts...)).To(Succeed())
				Expect(fileSize(diskFile)).To(Equal(expectSize))
			},
				Entry("grows the disk to the requested size", grownSize, raw.WithSize(grownSize)),
				Entry("keeps the source size when no size is requested", sourceSize),
			)

			It("rejects a size smaller than the source", func() {
				e := raw.Exec{}

				Expect(e.Create(diskFile, raw.WithSourceFile(imageSource), raw.WithSize(undersizedSize))).ToNot(Succeed())

				// localdisk.Apply skips creation for existing disks:
				Expect(diskFile).ToNot(BeAnExistingFile())
			})

			It("does not delete a disk it did not create", func() {
				e := raw.Exec{}
				existingDisk := []byte("disk written by someone else")
				Expect(os.WriteFile(diskFile, existingDisk, 0o600)).To(Succeed())

				Expect(e.Create(diskFile, raw.WithSourceFile(imageSource), raw.WithSize(undersizedSize))).ToNot(Succeed())

				Expect(os.ReadFile(diskFile)).To(Equal(existingDisk))
			})

			DescribeTable("invalid file size", func(size raw.WithSize) {
				e := raw.Exec{}

				Expect(e.Create(diskFile, raw.WithSourceFile(imageSource), size)).ToNot(Succeed())

				Expect(diskFile).ToNot(BeAnExistingFile())
			},
				Entry("negative size", raw.WithSize(-1)),
				Entry("zero", raw.WithSize(0)),
			)
		})

		When("the source file is a directory", func() {
			It("leaves no disk behind", func() {
				e := raw.Exec{}
				sourceDir := GinkgoT().TempDir()

				Expect(e.Create(diskFile, raw.WithSourceFile(sourceDir))).
					ToNot(Succeed())

				Expect(diskFile).ToNot(BeAnExistingFile(),
					"Create must delete the disk on failure, localdisk.Apply assumes disks are set up properly.")
			})
		})

		When("no source file set", func() {
			It("creates an empty disk of the requested size", func() {
				e := raw.Exec{}

				Expect(e.Create(diskFile, raw.WithSize(emptyDiskSize))).To(Succeed())
				Expect(fileSize(diskFile)).To(Equal(emptyDiskSize))
			})
			It("requires a size", func() {
				e := raw.Exec{}

				Expect(e.Create(diskFile)).ToNot(Succeed())
				Expect(diskFile).ToNot(BeAnExistingFile())
			})
		})

	})
})

func writeSourceFile(dir string) string {
	GinkgoHelper()

	sourceFile := filepath.Join(dir, "source.raw")

	Expect(os.WriteFile(sourceFile, make([]byte, sourceSize), 0o600)).To(Succeed())
	return sourceFile
}

func fileSize(filename string) int64 {
	GinkgoHelper()

	stat, err := os.Stat(filename)
	Expect(err).ToNot(HaveOccurred(), "stat %s", filename)

	return stat.Size()
}
