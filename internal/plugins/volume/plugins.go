// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package volume

import (
	"context"
	"fmt"
	"sync"

	"github.com/ironcore-dev/libvirt-provider/api"
	"k8s.io/apimachinery/pkg/util/sets"
)

type Host interface {
	PluginDir(pluginName string) string
	MachinePluginDir(machineID string, pluginName string) string
	MachineVolumeDir(machineID string, pluginName, volumeName string) string
}

type Plugin interface {
	Init(host Host) error
	Name() string
	GetBackingVolumeID(spec *api.VolumeSpec) (string, error)
	CanSupport(spec *api.VolumeSpec) bool

	Apply(ctx context.Context, spec *api.VolumeSpec, machine *api.Machine) (*Volume, error)
	Delete(ctx context.Context, computeVolumeName string, machineID string) error

	GetSize(ctx context.Context, spec *api.VolumeSpec) (int64, error)
}

type Volume struct {
	QCow2File string
	RawFile   string
	CephDisk  *CephDisk
	Handle    string
	Size      int64
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

func (m *PluginManager) FindPluginBySpec(volume *api.VolumeSpec) (Plugin, error) {
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
