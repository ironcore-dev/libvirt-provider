// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package isolated

import (
	"context"
	"os"

	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/pkg/plugins/networkinterface"
	providerhost "github.com/ironcore-dev/libvirt-provider/pkg/providerhost"
)

const (
	permFolder     = 0750
	pluginIsolated = "isolated"
)

type plugin struct {
	host providerhost.Host
}

func NewPlugin() providernetworkinterface.Plugin {
	return &plugin{}
}

func (p *plugin) Init(host providerhost.Host) error {
	p.host = host
	return nil
}

func (p *plugin) Apply(ctx context.Context, spec *api.NetworkInterfaceSpec, machine *api.Machine) (*providernetworkinterface.NetworkInterface, error) {
	if err := os.MkdirAll(p.host.MachineNetworkInterfaceDir(machine.ID, spec.Name), permFolder); err != nil {
		return nil, err
	}

	return &providernetworkinterface.NetworkInterface{
		Isolated: &providernetworkinterface.Isolated{},
	}, nil
}

func (p *plugin) Delete(ctx context.Context, computeNicName string, machineID string) error {
	return os.RemoveAll(p.host.MachineNetworkInterfaceDir(machineID, computeNicName))
}

func (p *plugin) Name() string {
	return pluginIsolated
}
