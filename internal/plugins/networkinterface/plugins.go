// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterface

import (
	"context"
	"errors"

	"github.com/ironcore-dev/libvirt-provider/api"
	providerhost "github.com/ironcore-dev/libvirt-provider/internal/host"
)

var ErrNotReady = errors.New("network interface not ready")

type EventHandler interface {
	HandleNICEvent(machineID string)
}

type EventHandlerFuncs struct {
	HandleNICEventFunc func(machineID string)
}

func (l EventHandlerFuncs) HandleNICEvent(machineID string) {
	if l.HandleNICEventFunc != nil {
		l.HandleNICEventFunc(machineID)
	}
}

type Plugin interface {
	Name() string
	Init(ctx context.Context, host providerhost.LibvirtHost) error
	AddEventHandler(handler EventHandler)

	Apply(ctx context.Context, spec *api.NetworkInterfaceSpec, machine *api.Machine) (*NetworkInterface, error)
	Delete(ctx context.Context, computeNicName string, machineID string) error
}

type NetworkInterface struct {
	Handle string

	Ips       []string
	Prefixes  []string
	VirtualIP string

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

// Direct renders
//
//	<interface type='direct'><source dev='<Dev>'mode='bridge'/></interface>.
//
// libvirt wraps the given device in a new macvtap (bridge mode) that qemu
// attaches to. Use this when the fd end of the tap in Dev is consumed by
// another process (e.g. a DPDK dataplane) and the guest should be coupled via
// the netdev end.
type Direct struct {
	Dev string
}
