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

package ceph

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/plugins/volume"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
)

const (
	pluginName = "libvirt-driver.api.onmetal.de/ceph"

	cephDriverName = "ceph"

	volumeAttributeImageKey     = "image"
	volumeAttributesMonitorsKey = "monitors"

	secretUserIDKey  = "userID"
	secretUserKeyKey = "userKey"
)

type plugin struct{}

type volumeData struct {
	monitors []api.CephMonitor
	image    string
	handle   string
	userID   string
	userKey  string
}

func NewPlugin() volume.Plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return pluginName
}

func (p *plugin) CanSupport(spec *ori.Volume) bool {
	connection := spec.Connection
	if connection == nil {
		return false
	}

	return connection.Driver == cephDriverName
}

func readSecret(secret map[string][]byte) (userID, userKey string, err error) {
	userIDData, ok := secret[secretUserIDKey]
	if !ok || len(userIDData) == 0 {
		return "", "", fmt.Errorf("no user id at %s", secretUserIDKey)
	}

	userKeyData, ok := secret[secretUserKeyKey]
	if !ok || len(userKeyData) == 0 {
		return "", "", fmt.Errorf("no user key at %s", secretUserKeyKey)
	}

	return string(userIDData), string(userKeyData), nil
}

func readVolumeAttributes(attrs map[string]string) (monitors []api.CephMonitor, image string, err error) {
	monitorsString, ok := attrs[volumeAttributesMonitorsKey]
	if !ok || monitorsString == "" {
		return nil, "", fmt.Errorf("no monitors data at %s", volumeAttributesMonitorsKey)
	}

	monitorsParts := strings.Split(monitorsString, ",")
	monitors = make([]api.CephMonitor, 0, len(monitorsParts))
	for _, monitorsPart := range monitorsParts {
		host, port, err := net.SplitHostPort(monitorsPart)
		if err != nil {
			return nil, "", fmt.Errorf("[monitor %s] error splitting host / port: %w", monitorsPart, err)
		}

		monitors = append(monitors, api.CephMonitor{Name: host, Port: port})
	}

	image, ok = attrs[volumeAttributeImageKey]
	if !ok || image == "" {
		return nil, "", fmt.Errorf("no image data at %s", volumeAttributeImageKey)
	}

	return monitors, image, nil
}

func (p *plugin) Prepare(spec *ori.Volume) (*api.VolumeSpec, error) {
	if spec.Connection == nil {
		return nil, fmt.Errorf("no connection spec")
	}

	volumeData, err := p.getVolumeData(spec.Connection)
	if err != nil {
		return nil, err
	}

	return &api.VolumeSpec{
		Provider: api.VolumeProviderCeph,
		CephDisk: &api.CephDisk{
			Image:    spec.Connection.Handle,
			Monitors: volumeData.monitors,
			Auth: api.CephAuthentication{
				UserName: volumeData.userID,
				UserKey:  volumeData.userKey,
			},
		},
	}, nil
}

func (p *plugin) Apply(ctx context.Context, spec *api.Volume) (*api.VolumeStatus, error) {
	return nil, nil
}

func (p *plugin) getVolumeData(spec *ori.VolumeConnection) (vData *volumeData, err error) {
	vData = new(volumeData)

	if spec.Driver != cephDriverName {
		return nil, fmt.Errorf("volume access specifies invalid driver %q", spec.Driver)
	}
	if spec.SecretData == nil {
		return nil, fmt.Errorf("volume access does not specify secret ref")
	}
	if spec.Handle == "" {
		return nil, fmt.Errorf("volume access does not specify handle")
	}
	vData.handle = spec.Handle

	vData.monitors, vData.image, err = readVolumeAttributes(spec.Attributes)
	if err != nil {
		return nil, fmt.Errorf("error reading volume attributes: %w", err)
	}

	vData.userID, vData.userKey, err = readSecret(spec.SecretData)
	if err != nil {
		return nil, fmt.Errorf("error reading secret: %w", err)
	}

	return vData, nil
}

func (p *plugin) Delete(ctx context.Context, volumeID string) error {
	return nil
}
