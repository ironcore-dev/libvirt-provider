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

package volume

import (
	"context"
	"fmt"
	"sync"

	computev1alpha1 "github.com/onmetal/onmetal-api/api/compute/v1alpha1"
	storagev1alpha1 "github.com/onmetal/onmetal-api/api/storage/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Spec struct {
	Compute *computev1alpha1.Volume
	Storage *storagev1alpha1.Volume
}

func (s *Spec) VolumeName() string {
	switch {
	case s.Compute != nil:
		return s.Compute.Name
	case s.Storage != nil:
		return s.Storage.Name
	default:
		return ""
	}
}

func referencedVolumeKey(namespace, machineName string, volume *computev1alpha1.Volume) (client.ObjectKey, bool) {
	switch {
	case volume.VolumeRef != nil:
		return client.ObjectKey{Namespace: namespace, Name: volume.VolumeRef.Name}, true
	case volume.Ephemeral != nil:
		return client.ObjectKey{Namespace: namespace, Name: computev1alpha1.MachineEphemeralVolumeName(machineName, volume.Name)}, true
	default:
		return client.ObjectKey{}, false
	}
}

func GetSpec(ctx context.Context, c client.Reader, namespace, machineName string, volume *computev1alpha1.Volume) (*Spec, error) {
	var (
		storage        *storagev1alpha1.Volume
		storageKey, ok = referencedVolumeKey(namespace, machineName, volume)
	)
	if ok {
		storage = &storagev1alpha1.Volume{}
		if err := c.Get(ctx, storageKey, storage); err != nil {
			return nil, err
		}
	}

	return &Spec{
		Compute: volume,
		Storage: storage,
	}, nil
}

type Host interface {
	Client() client.Client
	PluginDir(pluginName string) string
	MachinePluginDir(machineUID types.UID, pluginName string) string
	MachineVolumeDir(machineUID types.UID, pluginName, volumeName string) string
}

type Plugin interface {
	Init(host Host) error
	Name() string
	GetBackingVolumeID(spec *Spec) (string, error)
	CanSupport(spec *Spec) bool
	ConstructVolumeSpec(computeVolumeName string) (*Spec, error) // TODO: Reevaluate use of this method.

	Apply(ctx context.Context, spec *Spec, machine *computev1alpha1.Machine) (*Volume, error)
	Delete(ctx context.Context, computeVolumeName string, machineUID types.UID) error
}

type Volume struct {
	QCow2File string
	RawFile   string
	CephDisk  *CephDisk
	Handle    string
}

type CephDisk struct {
	Name       string
	Monitors   []CephMonitor
	Auth       *CephAuthentication
	Encryption *CephEncryption
}

type CephAuthentication struct {
	UserName string
	UserKey  string
}

type CephEncryption struct {
	EncryptionKey string
}

type CephMonitor struct {
	Name string
	Port string
}

type PluginManager struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

func NewPluginManager() *PluginManager {
	return &PluginManager{
		plugins: make(map[string]Plugin),
	}
}

func (m *PluginManager) InitPlugins(host Host, plugins []Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var initErrs []error
	for _, plugin := range plugins {
		name := plugin.Name()
		if _, ok := m.plugins[name]; ok {
			initErrs = append(initErrs, fmt.Errorf("[plugin %s] already registered", name))
			continue
		}

		if err := plugin.Init(host); err != nil {
			initErrs = append(initErrs, fmt.Errorf("[plugin %s] error initializing: %w", name, err))
			continue
		}

		m.plugins[name] = plugin
	}

	if len(initErrs) > 0 {
		return fmt.Errorf("error(s) initializing plugins: %v", initErrs)
	}
	return nil
}

func (m *PluginManager) FindPluginByName(name string) (Plugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin, ok := m.plugins[name]
	if !ok {
		return nil, fmt.Errorf("plugin %q not found", name)
	}
	return plugin, nil
}

func (m *PluginManager) FindPluginBySpec(volume *Spec) (Plugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matching []Plugin
	for _, plugin := range m.plugins {
		if plugin.CanSupport(volume) {
			matching = append(matching, plugin)
		}
	}
	switch len(matching) {
	case 0:
		return nil, fmt.Errorf("no plugin found supporting %#+v", volume)
	case 1:
		return matching[0], nil
	default:
		matchingNames := sets.NewString()
		for _, plugin := range matching {
			matchingNames.Insert(plugin.Name())
		}

		return nil, fmt.Errorf("multiple plugins matching for volume: %v", matchingNames.List())
	}
}
