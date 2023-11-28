// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package isolated

import (
	"context"
	"os"

	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	virtletnetworkinterface "github.com/ironcore-dev/libvirt-provider/pkg/plugins/networkinterface"
	virtlethost "github.com/ironcore-dev/libvirt-provider/pkg/virtlethost"
)

const (
	perm           = 0777
	pluginIsolated = "isolated"
)

type plugin struct {
	host virtlethost.Host
}

func NewPlugin() virtletnetworkinterface.Plugin {
	return &plugin{}
}

func (p *plugin) Init(host virtlethost.Host) error {
	p.host = host
	return nil
}

func (p *plugin) Apply(ctx context.Context, spec *api.NetworkInterfaceSpec, machine *api.Machine) (*virtletnetworkinterface.NetworkInterface, error) {
	if err := os.MkdirAll(p.host.MachineNetworkInterfaceDir(machine.ID, spec.Name), perm); err != nil {
		return nil, err
	}

	return &virtletnetworkinterface.NetworkInterface{
		Isolated: &virtletnetworkinterface.Isolated{},
	}, nil
}

func (p *plugin) Delete(ctx context.Context, computeNicName string, machineID string) error {
	return os.RemoveAll(p.host.MachineNetworkInterfaceDir(machineID, computeNicName))
}

func (p *plugin) Name() string {
	return pluginIsolated
}
