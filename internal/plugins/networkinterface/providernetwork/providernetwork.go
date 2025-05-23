// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package providernetwork

import (
	"context"
	"os"

	"github.com/ironcore-dev/libvirt-provider/api"
	providerhost "github.com/ironcore-dev/libvirt-provider/internal/host"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
)

const (
	perm              = 0777
	pluginProvidernet = "providernet"
)

type plugin struct {
	host providerhost.LibvirtHost
}

func NewPlugin() providernetworkinterface.Plugin {
	return &plugin{}
}

func (p *plugin) Init(host providerhost.LibvirtHost) error {
	p.host = host
	return nil
}

func (p *plugin) Apply(ctx context.Context, spec *api.NetworkInterfaceSpec, machine *api.Machine) (*providernetworkinterface.NetworkInterface, error) {
	if err := os.MkdirAll(p.host.MachineNetworkInterfaceDir(machine.ID, spec.Name), perm); err != nil {
		return nil, err
	}

	return &providernetworkinterface.NetworkInterface{
		ProviderNetwork: &providernetworkinterface.ProviderNetwork{
			NetworkName: spec.NetworkId,
		},
	}, nil
}

func (p *plugin) Delete(ctx context.Context, computeNicName string, machineID string) error {
	return os.RemoveAll(p.host.MachineNetworkInterfaceDir(machineID, computeNicName))
}

func (p *plugin) Name() string {
	return pluginProvidernet
}
