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

package networkinterface

import (
	"context"
	"fmt"

	"github.com/onmetal/libvirt-driver/pkg/api"
	virtlethost "github.com/onmetal/libvirt-driver/pkg/virtlethost"
	computev1alpha1 "github.com/onmetal/onmetal-api/api/compute/v1alpha1"
	networkingv1alpha1 "github.com/onmetal/onmetal-api/api/networking/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Spec struct {
	ComputeNetworkInterface *computev1alpha1.NetworkInterface
	NetworkInterface        *networkingv1alpha1.NetworkInterface
	Network                 *networkingv1alpha1.Network
	VirtualIP               *networkingv1alpha1.VirtualIP
}

func (s *Spec) NetworkInterfaceName() string {
	if s.ComputeNetworkInterface != nil {
		return s.ComputeNetworkInterface.Name
	}
	return s.NetworkInterface.Name
}

func networkInterfaceKey(namespace, machineName string, nic *computev1alpha1.NetworkInterface) (client.ObjectKey, error) {
	switch {
	case nic.NetworkInterfaceRef != nil:
		return client.ObjectKey{Namespace: namespace, Name: nic.NetworkInterfaceRef.Name}, nil
	case nic.Ephemeral != nil:
		return client.ObjectKey{Namespace: namespace, Name: computev1alpha1.MachineEphemeralNetworkInterfaceName(machineName, nic.Name)}, nil
	default:
		return client.ObjectKey{}, fmt.Errorf("unsupported compute network interface %#+v", nic)
	}
}

func virtualIPKey(namespace, networkInterfaceName string, vipSource *networkingv1alpha1.VirtualIPSource) (client.ObjectKey, error) {
	switch {
	case vipSource.VirtualIPRef != nil:
		return client.ObjectKey{Namespace: namespace, Name: vipSource.VirtualIPRef.Name}, nil
	case vipSource.Ephemeral != nil:
		return client.ObjectKey{Namespace: namespace, Name: networkingv1alpha1.NetworkInterfaceVirtualIPName(networkInterfaceName, *vipSource)}, nil
	default:
		return client.ObjectKey{}, fmt.Errorf("unsupported virtual ip source %#+v", vipSource)
	}
}

func GetSpec(ctx context.Context, c client.Reader, namespace, machineName string, computeNic *computev1alpha1.NetworkInterface) (*Spec, error) {
	nic := &networkingv1alpha1.NetworkInterface{}
	nicKey, err := networkInterfaceKey(namespace, machineName, computeNic)
	if err != nil {
		return nil, err
	}

	if err := c.Get(ctx, nicKey, nic); err != nil {
		return nil, err
	}

	network := &networkingv1alpha1.Network{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: nic.Spec.NetworkRef.Name}, network); err != nil {
		return nil, err
	}

	var virtualIP *networkingv1alpha1.VirtualIP
	if nic.Spec.VirtualIP != nil && nic.Status.VirtualIP != nil {
		virtualIPKey, err := virtualIPKey(namespace, nic.Name, nic.Spec.VirtualIP)
		if err != nil {
			return nil, err
		}

		virtualIP = &networkingv1alpha1.VirtualIP{}
		if err := c.Get(ctx, virtualIPKey, virtualIP); err != nil {
			return nil, err
		}
	}

	return &Spec{
		ComputeNetworkInterface: computeNic,
		NetworkInterface:        nic,
		Network:                 network,
		VirtualIP:               virtualIP,
	}, nil
}

type Plugin interface {
	Name() string
	Init(host virtlethost.Host) error

	Apply(ctx context.Context, spec *api.NetworkInterfaceSpec, machine *api.Machine) (*NetworkInterface, error)
	Delete(ctx context.Context, computeNicName string, machineID string) error
}

type NetworkInterface struct {
	Handle          string
	HostDevice      *HostDevice
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
