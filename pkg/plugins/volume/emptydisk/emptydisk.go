// Copyright 2022 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package emptydisk

import (
	"context"
	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/plugins/volume"
	"github.com/onmetal/libvirt-driver/pkg/qcow2"
	"github.com/onmetal/libvirt-driver/pkg/raw"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
)

const (
	pluginName = "libvirt-driver.api.onmetal.de/empty-disk"

	perm     = 0777
	filePerm = 0666
)

type plugin struct {
	qcow2 qcow2.QCow2
	raw   raw.Raw
}

func NewPlugin(qcow2 qcow2.QCow2, raw raw.Raw) volume.Plugin {
	return &plugin{
		qcow2: qcow2,
		raw:   raw,
	}
}

func (p *plugin) Name() string {
	return pluginName
}

func (p *plugin) CanSupport(spec *ori.Volume) bool {
	return spec.EmptyDisk != nil
}

func (p *plugin) Prepare(spec *ori.Volume) (*api.VolumeSpec, error) {
	return &api.VolumeSpec{
		Provider: api.VolumeProviderEmptyDisk,
		EmptyDisk: &api.EmptyDiskSpec{
			Size: spec.EmptyDisk.SizeBytes,
		},
	}, nil
}

func (p *plugin) Apply(ctx context.Context, spec *api.Volume) (*api.VolumeStatus, error) {

	return nil, nil
}

func (p *plugin) Delete(ctx context.Context, volumeID string) error {
	return nil
}
