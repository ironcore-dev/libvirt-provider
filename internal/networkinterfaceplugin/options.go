// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterfaceplugin

import (
	"fmt"
	"sort"

	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	"github.com/spf13/pflag"
)

type TypeOptions interface {
	PluginName() string
	AddFlags(fs *pflag.FlagSet)
	NetworkInterfacePlugin() (providernetworkinterface.Plugin, func(), error)
}

type TypeOptionsRegistry struct {
	nameToPluginOpts map[string]typeOptionsAndPriority
}

type typeOptionsAndPriority struct {
	TypeOptions
	priority int
}

func NewTypeOptionsRegistry() *TypeOptionsRegistry {
	return &TypeOptionsRegistry{
		nameToPluginOpts: make(map[string]typeOptionsAndPriority),
	}
}

func (r *TypeOptionsRegistry) Register(pluginOpts TypeOptions, priority int) error {
	pluginName := pluginOpts.PluginName()
	if _, ok := r.nameToPluginOpts[pluginName]; ok {
		return fmt.Errorf("plugin %q already registered", pluginName)
	}

	r.nameToPluginOpts[pluginName] = typeOptionsAndPriority{pluginOpts, priority}
	return nil
}

func (r *TypeOptionsRegistry) PluginNames() []string {
	names := make([]string, 0, len(r.nameToPluginOpts))
	for name := range r.nameToPluginOpts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return r.nameToPluginOpts[names[i]].priority < r.nameToPluginOpts[names[j]].priority
	})
	return names
}

func (r *TypeOptionsRegistry) DefaultPluginName() string {
	var (
		pluginName  string
		maxPriority *int
	)
	for name, entry := range r.nameToPluginOpts {
		if maxPriority == nil || *maxPriority > entry.priority {
			pluginName = name
			priority := entry.priority
			maxPriority = &priority
		}
	}
	return pluginName
}

func (r *TypeOptionsRegistry) ForeachPluginTypeOpts(f func(pluginName string, pluginOpts TypeOptions) bool) {
	for pluginName, pluginOpts := range r.nameToPluginOpts {
		if !f(pluginName, pluginOpts) {
			break
		}
	}
}

func (r *TypeOptionsRegistry) PluginTypeOptsByName(pluginName string) (TypeOptions, error) {
	pluginOpts, ok := r.nameToPluginOpts[pluginName]
	if !ok {
		return nil, fmt.Errorf("no plugin options for plugin name %q", pluginName)
	}

	return pluginOpts, nil
}

type Options struct {
	PluginName string
	registry   *TypeOptionsRegistry
}

func NewOptions(registry *TypeOptionsRegistry) *Options {
	return &Options{
		registry: registry,
	}
}

func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.PluginName, "network-interface-plugin-name", o.registry.DefaultPluginName(), fmt.Sprintf("Name of the network interface plugin to use. Available: %v", o.registry.PluginNames()))
	o.registry.ForeachPluginTypeOpts(func(pluginName string, pluginOpts TypeOptions) bool {
		pluginOpts.AddFlags(fs)
		return true
	})
}

func (o *Options) NetworkInterfacePlugin() (providernetworkinterface.Plugin, func(), error) {
	pluginOpts, err := o.registry.PluginTypeOptsByName(o.PluginName)
	if err != nil {
		return nil, nil, err
	}

	nicPlugin, cleanup, err := pluginOpts.NetworkInterfacePlugin()
	if err != nil {
		return nil, nil, err
	}

	return nicPlugin, cleanup, nil
}

var (
	DefaultPluginTypeRegistry = NewTypeOptionsRegistry()
)

func NewDefaultOptions() *Options {
	return NewOptions(DefaultPluginTypeRegistry)
}
