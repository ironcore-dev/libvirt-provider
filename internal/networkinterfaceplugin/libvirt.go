// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterfaceplugin

import (
	"context"

	"github.com/ironcore-dev/ironcore/utils/client/config"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/providernetwork"
	"github.com/spf13/pflag"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type libvirtNetworkOptions struct{}

func (o *libvirtNetworkOptions) AddFlags(_ *pflag.FlagSet) {}

func (o *libvirtNetworkOptions) PluginName() string {
	return "providernet"
}

func (o *libvirtNetworkOptions) NetworkInterfacePlugin(_ context.Context) (providernetworkinterface.Plugin, config.Controller, func(), error) {
	return providernetwork.NewPlugin(), nil, nil, nil
}

func init() {
	utilruntime.Must(DefaultPluginTypeRegistry.Register(&libvirtNetworkOptions{}, 10))
}
