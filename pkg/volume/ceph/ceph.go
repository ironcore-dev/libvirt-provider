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

	computev1alpha1 "github.com/onmetal/onmetal-api/api/compute/v1alpha1"
	storagev1alpha1 "github.com/onmetal/onmetal-api/api/storage/v1alpha1"
	"github.com/onmetal/virtlet/volume"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pluginName = "virtlet.api.onmetal.de/ceph"

	cephDriverName = "ceph"

	volumeAttributeImageKey     = "image"
	volumeAttributesMonitorsKey = "monitors"

	secretUserIDKey  = "userID"
	secretUserKeyKey = "userKey"

	secretEncryptionKey = "encryptionKey"
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

func (p *plugin) GetBackingVolumeID(spec *volume.Spec) (string, error) {
	storage := spec.Storage
	if storage == nil {
		return "", fmt.Errorf("volume is nil")
	}

	access := storage.Status.Access
	if access == nil {
		return "", fmt.Errorf("volume access is nil")
	}

	handle := access.Handle
	if handle == "" {
		return "", fmt.Errorf("volume access does not specify handle: %s", handle)
	}

	return fmt.Sprintf("%s^%s", pluginName, handle), nil
}

func (p *plugin) CanSupport(spec *volume.Spec) bool {
	storage := spec.Storage
	if storage == nil {
		return false
	}

	access := storage.Status.Access
	if access == nil {
		return false
	}

	return access.Driver == cephDriverName
}

func (p *plugin) ConstructVolumeSpec(volumeName string) (*volume.Spec, error) {
	return &volume.Spec{
		Compute: &computev1alpha1.Volume{
			Name: volumeName,
			VolumeSource: computev1alpha1.VolumeSource{
				VolumeRef: &corev1.LocalObjectReference{Name: volumeName},
			},
		},
		Storage: &storagev1alpha1.Volume{
			ObjectMeta: metav1.ObjectMeta{
				Name: volumeName, // TODO: Fix, this is wrong.
			},
		},
	}, nil
}

func readSecret(secret *corev1.Secret) (userID, userKey string, err error) {
	userIDData, ok := secret.Data[secretUserIDKey]
	if !ok || len(userIDData) == 0 {
		return "", "", fmt.Errorf("no user id at %s", secretUserIDKey)
	}

	userKeyData, ok := secret.Data[secretUserKeyKey]
	if !ok || len(userKeyData) == 0 {
		return "", "", fmt.Errorf("no user key at %s", secretUserKeyKey)
	}

	return string(userIDData), string(userKeyData), nil
}

func readEncryptionSecret(secret *corev1.Secret) (encryptionKey string, err error) {
	encryptionKeyData, ok := secret.Data[secretEncryptionKey]
	if !ok || len(encryptionKeyData) == 0 {
		return "", fmt.Errorf("no encryption key at %s", secretEncryptionKey)
	}

	return string(encryptionKeyData), nil
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

func (p *plugin) Apply(ctx context.Context, spec *volume.Spec, machine *computev1alpha1.Machine) (*volume.Volume, error) {
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

func (p *plugin) getVolumeData(ctx context.Context, spec *volume.Spec, machine *computev1alpha1.Machine) (vData *volumeData, err error) {
	vData = new(volumeData)
	access := spec.Storage.Status.Access
	if access == nil {
		return nil, fmt.Errorf("volume does not specify access")
	}
	if access.Driver != cephDriverName {
		return nil, fmt.Errorf("volume access specifies invalid driver %q", access.Driver)
	}
	if access.SecretRef == nil {
		return nil, fmt.Errorf("volume access does not specify secret ref")
	}
	if access.Handle == "" {
		return nil, fmt.Errorf("volume access does not specify handle")
	}
	vData.handle = access.Handle

	vData.monitors, vData.image, err = readVolumeAttributes(access.VolumeAttributes)
	if err != nil {
		return nil, fmt.Errorf("error reading volume attributes: %w", err)
	}

	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: machine.Namespace, Name: access.SecretRef.Name}
	if err := p.host.Client().Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("error getting secret: %w", err)
	}

	vData.userID, vData.userKey, err = readSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("error reading secret: %w", err)
	}

	storageSpec := spec.Storage.Spec
	if storageSpec.Encryption != nil {
		encryptionSecret := &corev1.Secret{}
		encryptionSecretKey := client.ObjectKey{Namespace: machine.Namespace, Name: storageSpec.Encryption.SecretRef.Name}
		if err := p.host.Client().Get(ctx, encryptionSecretKey, encryptionSecret); err != nil {
			return nil, fmt.Errorf("error getting encryption secret: %w", err)
		}
		vData.encryptionKey, err = readEncryptionSecret(encryptionSecret)
		if err != nil {
			return nil, fmt.Errorf("error reading secret: %w", err)
		}
	}

	return vData, nil
}

func (p *plugin) Delete(ctx context.Context, computeVolumeName string, machineUID types.UID) error {
	return nil
}
