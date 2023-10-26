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

	"github.com/onmetal/libvirt-driver/pkg/api"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

type Plugin interface {
	Name() string
	CanSupport(spec *ori.Volume) bool

	Prepare(spec *ori.Volume) (*api.VolumeSpec, error)
	Apply(ctx context.Context, spec *api.Volume) (*api.VolumeStatus, error)
	Delete(ctx context.Context, volumeID string, machineID types.UID) error
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

func (m *PluginManager) InitPlugins(plugins []Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var initErrs []error
	for _, plugin := range plugins {
		name := plugin.Name()
		if _, ok := m.plugins[name]; ok {
			initErrs = append(initErrs, fmt.Errorf("[plugin %s] already registered", name))
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

func (m *PluginManager) FindPluginBySpec(volume *ori.Volume) (Plugin, error) {
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
