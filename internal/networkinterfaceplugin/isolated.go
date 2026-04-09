// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterfaceplugin

import (
	"context"

	"github.com/ironcore-dev/ironcore/utils/client/config"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/isolated"
	"github.com/spf13/pflag"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type isolatedOptions struct{}

func (o *isolatedOptions) AddFlags(_ *pflag.FlagSet) {}

func (o *isolatedOptions) PluginName() string {
	return "isolated"
}

func (o *isolatedOptions) NetworkInterfacePlugin(_ context.Context) (providernetworkinterface.Plugin, config.Controller, func(), error) {
	return isolated.NewPlugin(), nil, nil, nil
}

func init() {
	utilruntime.Must(DefaultPluginTypeRegistry.Register(&isolatedOptions{}, 5))
}
