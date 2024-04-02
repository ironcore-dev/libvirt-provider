// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterfaceplugin

import (
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/providernetwork"
	"github.com/spf13/pflag"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type libvirtNetworkOptions struct{}

func (o *libvirtNetworkOptions) AddFlags(fs *pflag.FlagSet) {}

func (o *libvirtNetworkOptions) PluginName() string {
	return "providernet"
}

func (o *libvirtNetworkOptions) NetworkInterfacePlugin() (providernetworkinterface.Plugin, func(), error) {
	return providernetwork.NewPlugin(), nil, nil
}

func init() {
	utilruntime.Must(DefaultPluginTypeRegistry.Register(&libvirtNetworkOptions{}, 10))
}
