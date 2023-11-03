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

package networkinterfaceplugin

import (
	"fmt"
	"sort"

	virtletnetworkinterface "github.com/onmetal/libvirt-driver/pkg/plugins/networkinterface"
	"github.com/spf13/pflag"
)

type TypeOptions interface {
	PluginName() string
	AddFlags(fs *pflag.FlagSet)
	NetworkInterfacePlugin() (virtletnetworkinterface.Plugin, func(), error)
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

func (o *Options) NetworkInterfacePlugin() (virtletnetworkinterface.Plugin, func(), error) {
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
