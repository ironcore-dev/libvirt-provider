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

package isolated

import (
	"context"
	"os"

	"github.com/onmetal/libvirt-driver/pkg/api"
	virtletnetworkinterface "github.com/onmetal/libvirt-driver/pkg/plugins/networkinterface"
	virtlethost "github.com/onmetal/libvirt-driver/pkg/virtlethost"
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
