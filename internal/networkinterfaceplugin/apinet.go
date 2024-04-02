// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterfaceplugin

import (
	"fmt"

	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/apinet"
	"github.com/spf13/pflag"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type apinetOptions struct {
	APInetNodeName string
}

func (o *apinetOptions) PluginName() string {
	return "apinet"
}

func (o *apinetOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.APInetNodeName, "apinet-node-name", "", "APInet node name")
}

func (o *apinetOptions) NetworkInterfacePlugin() (providernetworkinterface.Plugin, func(), error) {
	if o.APInetNodeName == "" {
		return nil, nil, fmt.Errorf("must specify apinet-node-name")
	}

	return apinet.NewPlugin(o.APInetNodeName), nil, nil
}

func init() {
	utilruntime.Must(DefaultPluginTypeRegistry.Register(&apinetOptions{}, 1))
}
