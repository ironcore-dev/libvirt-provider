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

package ceph

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/plugins/volume"
)

const (
	pluginName = "libvirt-provider.ironcore.dev/ceph"

	cephDriverName = "ceph"

	volumeAttributeImageKey     = "image"
	volumeAttributesMonitorsKey = "monitors"

	secretUserIDKey  = "userID"
	secretUserKeyKey = "userKey"

	//secretEncryptionKey = "encryptionKey"
)

type plugin struct {
	host volume.Host
}

type volumeData struct {
	monitors      []volume.CephMonitor
	image         string
	handle        string
	userID        string
	userKey       string
	encryptionKey string
}

func NewPlugin() volume.Plugin {
	return &plugin{}
}

func (p *plugin) Init(host volume.Host) error {
	p.host = host
	return nil
}

func (p *plugin) Name() string {
	return pluginName
}

func (p *plugin) GetBackingVolumeID(spec *api.VolumeSpec) (string, error) {
	storage := spec.Connection
	if storage == nil {
		return "", fmt.Errorf("volume is nil")
	}

	handle := storage.Handle
	if handle == "" {
		return "", fmt.Errorf("volume access does not specify handle: %s", handle)
	}

	return fmt.Sprintf("%s^%s", pluginName, handle), nil
}

func (p *plugin) CanSupport(spec *api.VolumeSpec) bool {
	storage := spec.Connection
	if storage == nil {
		return false
	}

	return storage.Driver == cephDriverName
}

func readSecretData(data map[string][]byte) (userID, userKey string, err error) {
	userIDData, ok := data[secretUserIDKey]
	if !ok || len(userIDData) == 0 {
		return "", "", fmt.Errorf("no user id at %s", secretUserIDKey)
	}

	userKeyData, ok := data[secretUserKeyKey]
	if !ok || len(userKeyData) == 0 {
		return "", "", fmt.Errorf("no user key at %s", secretUserKeyKey)
	}

	return string(userIDData), string(userKeyData), nil
}

func readVolumeAttributes(attrs map[string]string) (monitors []volume.CephMonitor, image string, err error) {
	monitorsString, ok := attrs[volumeAttributesMonitorsKey]
	if !ok || monitorsString == "" {
		return nil, "", fmt.Errorf("no monitors data at %s", volumeAttributesMonitorsKey)
	}

	monitorsParts := strings.Split(monitorsString, ",")
	monitors = make([]volume.CephMonitor, 0, len(monitorsParts))
	for _, monitorsPart := range monitorsParts {
		host, port, err := net.SplitHostPort(monitorsPart)
		if err != nil {
			return nil, "", fmt.Errorf("[monitor %s] error splitting host / port: %w", monitorsPart, err)
		}

		monitors = append(monitors, volume.CephMonitor{Name: host, Port: port})
	}

	image, ok = attrs[volumeAttributeImageKey]
	if !ok || image == "" {
		return nil, "", fmt.Errorf("no image data at %s", volumeAttributeImageKey)
	}

	return monitors, image, nil
}

func (p *plugin) Apply(ctx context.Context, spec *api.VolumeSpec, machine *api.Machine) (*volume.Volume, error) {
	volumeData, err := p.getVolumeData(ctx, spec, machine)
	if err != nil {
		return nil, err
	}

	return &volume.Volume{
		QCow2File: "",
		RawFile:   "",
		CephDisk: &volume.CephDisk{
			Name:     volumeData.image,
			Monitors: volumeData.monitors,
			Auth: &volume.CephAuthentication{
				UserName: volumeData.userID,
				UserKey:  volumeData.userKey,
			},
			Encryption: &volume.CephEncryption{
				EncryptionKey: volumeData.encryptionKey,
			},
		},
		Handle: volumeData.handle,
	}, nil
}

func (p *plugin) getVolumeData(ctx context.Context, spec *api.VolumeSpec, machine *api.Machine) (vData *volumeData, err error) {
	vData = new(volumeData)
	connection := spec.Connection
	if connection == nil {
		return nil, fmt.Errorf("volume does not specify connection")
	}
	if connection.Driver != cephDriverName {
		return nil, fmt.Errorf("volume connection specifies invalid driver %q", connection.Driver)
	}
	if connection.Attributes == nil {
		return nil, fmt.Errorf("volume connection does not specify attributes")
	}
	if connection.SecretData == nil {
		return nil, fmt.Errorf("volume connection does not specify secret data")
	}
	if connection.Handle == "" {
		return nil, fmt.Errorf("volume connection does not specify handle")
	}
	vData.handle = connection.Handle

	vData.monitors, vData.image, err = readVolumeAttributes(connection.Attributes)
	if err != nil {
		return nil, fmt.Errorf("error reading volume attributes: %w", err)
	}

	vData.userID, vData.userKey, err = readSecretData(connection.SecretData)
	if err != nil {
		return nil, fmt.Errorf("error reading secret: %w", err)
	}

	//TODO currently not supported in ORI
	//storageSpec := spec.Storage.Spec
	//if storageSpec.Encryption != nil {
	//	encryptionSecret := &corev1.Secret{}
	//	encryptionSecretKey := client.ObjectKey{Namespace: machine.Namespace, Name: storageSpec.Encryption.SecretRef.Name}
	//	if err := p.host.Client().Get(ctx, encryptionSecretKey, encryptionSecret); err != nil {
	//		return nil, fmt.Errorf("error getting encryption secret: %w", err)
	//	}
	//	vData.encryptionKey, err = readEncryptionSecret(encryptionSecret)
	//	if err != nil {
	//		return nil, fmt.Errorf("error reading secret: %w", err)
	//	}
	//}

	return vData, nil
}

func (p *plugin) Delete(ctx context.Context, computeVolumeName string, machineID string) error {
	return nil
}
