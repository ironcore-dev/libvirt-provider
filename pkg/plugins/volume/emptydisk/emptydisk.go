// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package emptydisk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/plugins/volume"
	"github.com/onmetal/libvirt-driver/pkg/qcow2"
	"github.com/onmetal/libvirt-driver/pkg/raw"
	utilstrings "k8s.io/utils/strings"
)

const (
	pluginName = "libvirt-driver.api.onmetal.de/empty-disk"

	perm     = 0777
	filePerm = 0666
)

type plugin struct {
	host  volume.Host
	qcow2 qcow2.QCow2
	raw   raw.Raw
}

func NewPlugin(qcow2 qcow2.QCow2, raw raw.Raw) volume.Plugin {
	return &plugin{
		qcow2: qcow2,
		raw:   raw,
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
	if volume.EmptyDisk == nil {
		return "", fmt.Errorf("volume does not specify an EmptyDisk")
	}
	return volume.Name, nil
}

func (p *plugin) CanSupport(volume *api.VolumeSpec) bool {
	return volume.EmptyDisk != nil
}

func (p *plugin) diskFilename(computeVolumeName string, machineID string) string {
	return filepath.Join(p.host.MachineVolumeDir(machineID, utilstrings.EscapeQualifiedName(pluginName), computeVolumeName), "disk.raw")
}

func (p *plugin) Apply(ctx context.Context, spec *api.VolumeSpec, machine *api.Machine) (*volume.Volume, error) {
	volumeDir := p.host.MachineVolumeDir(machine.ID, utilstrings.EscapeQualifiedName(pluginName), spec.Name)
	if err := os.MkdirAll(volumeDir, perm); err != nil {
		return nil, err
	}

	handle, err := randomHex(8)
	if err != nil {
		return nil, fmt.Errorf("failed to generate WWN/handle for the disk: %w", err)
	}

	diskFilename := p.diskFilename(spec.Name, machine.ID)
	if _, err := os.Stat(diskFilename); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("error stat-ing disk: %w", err)
		}

		var size int64 = 500 * 1024 * 1024 // 500Mi by default
		if sizeLimit := spec.EmptyDisk.Size; sizeLimit != 0 {
			size = sizeLimit
		}

		if err := p.raw.Create(diskFilename, raw.WithSize(size)); err != nil {
			return nil, fmt.Errorf("error creating disk %w", err)
		}
		if err := os.Chmod(diskFilename, filePerm); err != nil {
			return nil, fmt.Errorf("error changing disk file mode: %w", err)
		}
	}
	return &volume.Volume{RawFile: diskFilename, Handle: handle}, nil
}

func (p *plugin) Delete(ctx context.Context, computeVolumeName string, machineID string) error {
	return os.RemoveAll(p.host.MachineVolumeDir(machineID, utilstrings.EscapeQualifiedName(pluginName), computeVolumeName))
}

// randomHex generates random hexadecimal digits of the length n*2.
func randomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
