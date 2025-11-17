// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package localdisk

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/osutils"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/volume"
	"github.com/ironcore-dev/libvirt-provider/internal/raw"
	ociutils "github.com/ironcore-dev/provider-utils/ociutils/oci"
	utilstrings "k8s.io/utils/strings"
)

const (
	pluginName = "libvirt-provider.ironcore.dev/local-disk"

	defaultSize = 500 * 1024 * 1024 // 500Mi by default

	perm     = 0777
	filePerm = 0666
)

type plugin struct {
	host volume.Host
	raw  raw.Raw

	imageCache ociutils.Cache
}

func NewPlugin(raw raw.Raw, osImages ociutils.Cache) volume.Plugin {
	return &plugin{
		raw:        raw,
		imageCache: osImages,
	}
}

func (p *plugin) Init(host volume.Host) error {
	p.host = host
	return nil
}

func (p *plugin) Name() string {
	return pluginName
}

func (p *plugin) GetBackingVolumeID(volume *api.VolumeSpec) (string, error) {
	if volume.LocalDisk == nil {
		return "", fmt.Errorf("volume does not specify an LocalDisk")
	}
	return volume.Name, nil
}

func (p *plugin) CanSupport(volume *api.VolumeSpec) bool {
	return volume.LocalDisk != nil
}

func (p *plugin) diskFilename(computeVolumeName string, machineID string) string {
	return filepath.Join(p.host.MachineVolumeDir(machineID, utilstrings.EscapeQualifiedName(pluginName), computeVolumeName), "disk.raw")
}

func (p *plugin) Apply(ctx context.Context, spec *api.VolumeSpec, machine *api.Machine) (*volume.Volume, error) {
	log := logr.FromContextOrDiscard(ctx)

	volumeDir := p.host.MachineVolumeDir(machine.ID, utilstrings.EscapeQualifiedName(pluginName), spec.Name)

	log.V(2).Info("Creating volume directory", "directory", volumeDir)
	if err := os.MkdirAll(volumeDir, perm); err != nil {
		return nil, err
	}

	diskFilename := p.diskFilename(spec.Name, machine.ID)

	ok, err := osutils.RegularFileExists(diskFilename)
	if err != nil {
		log.V(2).Info("Disk already exists", "file", diskFilename)
		return nil, err
	}

	if !ok {
		var createOption raw.CreateOption
		if imgRef := spec.LocalDisk.Image; imgRef != nil {
			img, err := p.imageCache.Get(ctx, *imgRef)
			if err != nil {
				return nil, err
			}

			log.V(2).Info("Create disk with rootfs from img", "file", img.RootFS.Path)
			createOption = raw.WithSourceFile(img.RootFS.Path)
		} else {
			size := spec.LocalDisk.Size
			if size == 0 {
				size = defaultSize
			}

			log.V(2).Info("Create disk", "size", size)
			createOption = raw.WithSize(size)
		}

		if err := p.raw.Create(diskFilename, createOption); err != nil {
			return nil, fmt.Errorf("error creating disk %w", err)
		}
		if err := os.Chmod(diskFilename, filePerm); err != nil {
			return nil, fmt.Errorf("error changing disk file mode: %w", err)
		}
	}

	stat, err := os.Stat(diskFilename)
	if err != nil {
		return nil, fmt.Errorf("error checking disk file size: %w", err)
	}

	return &volume.Volume{
		RawFile:                   diskFilename,
		Handle:                    generateWWN(machine.ID, spec.Name),
		EffectiveStorageBytesSize: stat.Size(),
	}, nil
}

func (p *plugin) Delete(ctx context.Context, computeVolumeName string, machineID string) error {
	return os.RemoveAll(p.host.MachineVolumeDir(machineID, utilstrings.EscapeQualifiedName(pluginName), computeVolumeName))
}

func generateWWN(machineID, diskName string) string {
	input := fmt.Sprintf("%s:%s", machineID, diskName)
	hash := sha1.Sum([]byte(input))
	wwnBytes := hash[:8]

	wwnBytes[0] |= 0x80

	return strings.ToUpper(hex.EncodeToString(wwnBytes))
}
