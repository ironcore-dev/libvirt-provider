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
	virtletnetworkinterface "github.com/ironcore-dev/libvirt-provider/pkg/plugins/networkinterface"
	"github.com/ironcore-dev/libvirt-provider/pkg/plugins/networkinterface/providernetwork"
	"github.com/spf13/pflag"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type libvirtNetworkOptions struct{}

func (o *libvirtNetworkOptions) AddFlags(fs *pflag.FlagSet) {}

func (o *libvirtNetworkOptions) PluginName() string {
	return "providernet"
}

func (o *libvirtNetworkOptions) NetworkInterfacePlugin() (virtletnetworkinterface.Plugin, func(), error) {
	return providernetwork.NewPlugin(), nil, nil
}

func init() {
	utilruntime.Must(DefaultPluginTypeRegistry.Register(&libvirtNetworkOptions{}, 10))
}
