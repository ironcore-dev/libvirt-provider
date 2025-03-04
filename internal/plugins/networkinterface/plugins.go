// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterface

import (
	"context"

	"github.com/ironcore-dev/libvirt-provider/api"
	providerhost "github.com/ironcore-dev/libvirt-provider/internal/host"
)

type Plugin interface {
	Name() string
	Init(host providerhost.Host) error

	Apply(ctx context.Context, spec *api.NetworkInterfaceSpec, machine *api.Machine) (*NetworkInterface, error)
	Delete(ctx context.Context, computeNicName string, machineID string) error
}

type NetworkInterface struct {
	Handle          string
	HostDevice      *HostDevice
	Direct          *Direct
	Isolated        *Isolated
	ProviderNetwork *ProviderNetwork
}

type Isolated struct{}

type ProviderNetwork struct {
	NetworkName string
}

type HostDevice struct {
	Domain   uint
	Bus      uint
	Slot     uint
	Function uint
}

type Direct struct {
	Dev string
}
