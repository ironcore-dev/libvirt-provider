// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package localdisk_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/volume"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/volume/localdisk"
	"github.com/ironcore-dev/libvirt-provider/internal/raw"
	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"
	ociutils "github.com/ironcore-dev/provider-utils/ociutils/oci"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	machineID          = "1a2b3c"
	volumeName         = "disk-1"
	imageSize    int64 = 4096
	negativeSize int64 = -1
)

var _ = Describe("Plugin", func() {
	Describe("Apply", func() {
		var volumeDir, diskFile string
		var plugin volume.Plugin

		BeforeEach(func() {
			volumeDir = GinkgoT().TempDir()
			diskFile = filepath.Join(volumeDir, "disk.raw")

			plugin = localdisk.NewPlugin(raw.Exec{}, fakeImageCache{rootFSPath: writeImageRootFS()})
			Expect(plugin.Init(fakeHost{volumeDir: volumeDir})).To(Succeed())
		})

		When("local disk with image", func() {
			It("rejects a negative size", func(ctx SpecContext) {
				_, err := plugin.Apply(ctx, imageBackedVolume(negativeSize), &api.Machine{
					Metadata: apiutils.Metadata{ID: machineID},
				})

				Expect(err).To(HaveOccurred())

				Expect(diskFile).ToNot(BeAnExistingFile(),
					"disk with the wrong size gets stuck and will not be fixed on reapply")
			})
		})
	})
})

func imageBackedVolume(size int64) *api.VolumeSpec {
	image := "example.org/os-foo/gardenlinux:latest"

	return &api.VolumeSpec{
		Name: volumeName,
		LocalDisk: &api.LocalDiskSpec{
			Size:  size,
			Image: &image,
		},
	}
}

func writeImageRootFS() string {
	GinkgoHelper()

	rootFS := filepath.Join(GinkgoT().TempDir(), "rootfs.raw")

	Expect(os.WriteFile(rootFS, make([]byte, imageSize), 0o600)).To(Succeed())
	return rootFS
}

type fakeHost struct {
	volumeDir string
}

func (h fakeHost) PluginDir(string) string                        { return h.volumeDir }
func (h fakeHost) MachinePluginDir(string, string) string         { return h.volumeDir }
func (h fakeHost) MachineVolumeDir(string, string, string) string { return h.volumeDir }

type fakeImageCache struct {
	rootFSPath string
}

func (c fakeImageCache) Get(context.Context, string) (*ociutils.Image, error) {
	return &ociutils.Image{RootFS: &ociutils.FileLayer{Path: c.rootFSPath}}, nil
}

func (fakeImageCache) AddListener(ociutils.Listener) {}
