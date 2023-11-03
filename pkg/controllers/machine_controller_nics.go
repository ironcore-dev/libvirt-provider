// Copyright 2023 OnMetal authors
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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	"github.com/onmetal/libvirt-driver/pkg/api"
	virtletnetworkinterface "github.com/onmetal/libvirt-driver/pkg/plugins/networkinterface"
	virtlethost "github.com/onmetal/libvirt-driver/pkg/virtlethost"
	"k8s.io/apimachinery/pkg/util/sets"
	"libvirt.org/go/libvirtxml"
)

var (
	errNoNetworkInterfaceAlias = errors.New("no network interface alias")
)

func (r *MachineReconciler) setDomainNetworkInterfaces(
	ctx context.Context,
	machine *api.Machine,
	domainDesc *libvirtxml.Domain,
) ([]api.NetworkInterfaceStatus, error) {
	machineNics, err := virtlethost.ReadMachineNetworkInterfaces(r.host, machine.ID)
	if err != nil {
		return nil, err
	}

	var (
		specNicNames = sets.NewString()
		states       []api.NetworkInterfaceStatus
	)
	for _, nic := range machine.Spec.NetworkInterfaces {
		specNicNames.Insert(nic.Name)

		virtletNic, err := r.networkInterfacePlugin.Apply(ctx, nic, machine)
		if err != nil {
			return nil, fmt.Errorf("[network interface %s] %w", nic.Name, err)
		}

		libvirtNic, err := virtletNetworkInterfaceToLibvirt(nic.Name, virtletNic)
		if err != nil {
			return nil, fmt.Errorf("[network interface %s] %w", nic.Name, err)
		}

		switch {
		case libvirtNic.hostDev != nil:
			addDomainHostdev(domainDesc, *libvirtNic.hostDev)
		case libvirtNic.iface != nil:
			addDomainInterface(domainDesc, *libvirtNic.iface)
		default:
			return nil, fmt.Errorf("[network interface %s] unsupported by libvirt", nic.Name)
		}

		states = append(states, api.NetworkInterfaceStatus{
			Name:   nic.Name,
			Handle: virtletNic.Handle,
			State:  api.NetworkInterfaceStateAttached,
		})
	}

	for _, machineNic := range machineNics {
		if specNicNames.Has(machineNic.NetworkInterfaceName) {
			continue
		}

		if err := r.networkInterfacePlugin.Delete(ctx, machineNic.NetworkInterfaceName, machine.ID); err != nil {
			return nil, fmt.Errorf("[network interface %s] %w", machineNic.NetworkInterfaceName, err)
		}
	}
	return states, nil
}

func addDomainHostdev(domainDesc *libvirtxml.Domain, hostdev libvirtxml.DomainHostdev) {
	if domainDesc.Devices == nil {
		domainDesc.Devices = &libvirtxml.DomainDeviceList{}
	}

	domainDesc.Devices.Hostdevs = append(domainDesc.Devices.Hostdevs, hostdev)
}

func addDomainInterface(domainDesc *libvirtxml.Domain, iface libvirtxml.DomainInterface) {
	if domainDesc.Devices == nil {
		domainDesc.Devices = &libvirtxml.DomainDeviceList{}
	}

	domainDesc.Devices.Interfaces = append(domainDesc.Devices.Interfaces, iface)
}

func (r *MachineReconciler) attachDetachNetworkInterfaces(
	ctx context.Context,
	log logr.Logger,
	machine *api.Machine,
	domainDesc *libvirtxml.Domain,
) ([]api.NetworkInterfaceStatus, error) {
	domain := machineDomain(machine.ID)

	machineNicByName, err := r.listMachineNetworkInterfaces(machine.ID)
	if err != nil {
		return nil, err
	}

	mountedNics, err := r.computeMountedNetworkInterfaces(domainDesc)
	if err != nil {
		return nil, err
	}

	desiredNics := r.desiredNetworkInterfaces(machine)

	var (
		nicStates []api.NetworkInterfaceStatus
		errs      []error
	)

	for nicName, actualNic := range mountedNics {
		if _, ok := desiredNics[nicName]; ok {
			continue
		}

		log.V(1).Info("Detaching network interface", "NetworkInterfaceName", nicName)
		if err := r.detachDomainDevice(domain, actualNic.libvirt.device()); err != nil {
			errs = append(errs, fmt.Errorf("[network interface %s] error detaching: %w", nicName, err))
		} else {
			log.V(1).Info("Successfully detached network interface", "NetworkInterfaceName", nicName)
			delete(mountedNics, nicName)
		}
	}

	for nicName, desiredNic := range desiredNics {
		log.V(1).Info("Reconciling desired network interface", "NetworkInterfaceName", nicName)
		mountedNic, err := r.reconcileDesiredNetworkInterface(ctx, machine, domain, mountedNics, desiredNic)
		if err != nil {
			errs = append(errs, fmt.Errorf("[network interface %s] error reconciling: %w", nicName, err))
		} else {
			log.V(1).Info("Successfully reconciled desired network interface", "NetworkInterfaceName", nicName)
			mountedNics[nicName] = *mountedNic
			nicStates = append(nicStates, api.NetworkInterfaceStatus{
				Name:   nicName,
				Handle: mountedNic.networkInterface.Handle,
				State:  api.NetworkInterfaceStateAttached,
			})
		}
	}

	for nicName, machineNic := range machineNicByName {
		if _, ok := mountedNics[nicName]; ok {
			continue
		}

		log.V(1).Info("Tearing down network interface", "NetworkInterfaceName", nicName)
		if err := r.deleteNetworkInterface(ctx, machine, machineNic); err != nil {
			errs = append(errs, fmt.Errorf("[network interface %s] error deleting: %w", nicName, err))
		} else {
			log.V(1).Info("Successfully torn down network interface", "NetworkInterfaceName", nicName)
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("attach / detach error(s): %v", errs)
	}
	return nicStates, nil
}

func (r *MachineReconciler) deleteNetworkInterface(
	ctx context.Context,
	machine *api.Machine,
	nic virtlethost.MachineNetworkInterface,
) error {
	return r.networkInterfacePlugin.Delete(ctx, nic.NetworkInterfaceName, machine.ID)
}

func (r *MachineReconciler) reconcileDesiredNetworkInterface(
	ctx context.Context,
	machine *api.Machine,
	domain libvirt.Domain,
	mountedNics map[string]mountedNetworkInterface,
	nic *api.NetworkInterfaceSpec,
) (*mountedNetworkInterface, error) {
	virtletNic, err := r.networkInterfacePlugin.Apply(ctx, nic, machine)
	if err != nil {
		return nil, err
	}

	mountedNic, ok := mountedNics[nic.Name]
	mountedNic.networkInterface.Handle = virtletNic.Handle
	if ok && reflect.DeepEqual(mountedNic.networkInterface, virtletNic) {
		return &mountedNic, nil
	}
	if ok {
		if err := r.detachDomainDevice(domain, mountedNic.libvirt.device()); err != nil {
			return nil, err
		}
	}

	libvirtNic, err := virtletNetworkInterfaceToLibvirt(nic.Name, virtletNic)
	if err != nil {
		return nil, err
	}

	if err := r.attachDomainDevice(domain, libvirtNic.device()); err != nil {
		return nil, fmt.Errorf("error attaching network interface device: %w", err)
	}
	return &mountedNetworkInterface{
		networkInterface: virtletNic,
		libvirt:          libvirtNic,
	}, nil
}

func (r *MachineReconciler) listMachineNetworkInterfaces(machineUID string) (map[string]virtlethost.MachineNetworkInterface, error) {
	machineNics, err := virtlethost.ReadMachineNetworkInterfaces(r.host, machineUID)
	if err != nil {
		return nil, err
	}

	res := make(map[string]virtlethost.MachineNetworkInterface, len(machineNics))
	for _, machineVolume := range machineNics {
		res[machineVolume.NetworkInterfaceName] = machineVolume
	}
	return res, nil
}

type mountedNetworkInterface struct {
	networkInterface *virtletnetworkinterface.NetworkInterface
	libvirt          *libvirtNetworkInterface
}

type libvirtNetworkInterface struct {
	hostDev *libvirtxml.DomainHostdev
	iface   *libvirtxml.DomainInterface
}

func (i *libvirtNetworkInterface) device() libvirtxml.Document {
	switch {
	case i.hostDev != nil:
		return i.hostDev
	case i.iface != nil:
		return i.iface
	default:
		return nil
	}
}

func (r *MachineReconciler) computeMountedNetworkInterfaces(domainDesc *libvirtxml.Domain) (map[string]mountedNetworkInterface, error) {
	res := make(map[string]mountedNetworkInterface)
	for _, hostDev := range domainDescHostDevices(domainDesc) {
		if hostDev.Alias == nil || !strings.HasPrefix(hostDev.Alias.Name, networkInterfaceAliasPrefix) {
			continue
		}

		name, err := parseNetworkInterfaceAlias(hostDev.Alias.Name)
		if err != nil {
			return nil, err
		}

		hostDev := hostDev
		nic, err := libvirtHostdevToVirtletNetworkInterface(&hostDev)
		if err != nil {
			return nil, err
		}

		res[name] = mountedNetworkInterface{
			networkInterface: nic,
			libvirt: &libvirtNetworkInterface{
				hostDev: &hostDev,
			},
		}
	}
	for _, iface := range domainDescInterfaces(domainDesc) {
		if iface.Alias == nil || !strings.HasPrefix(iface.Alias.Name, networkInterfaceAliasPrefix) {
			continue
		}

		name, err := parseNetworkInterfaceAlias(iface.Alias.Name)
		if err != nil {
			return nil, err
		}

		iface := iface
		nic, err := libvirtInterfaceToVirtletNetworkInterface(&iface)
		if err != nil {
			return nil, err
		}

		res[name] = mountedNetworkInterface{
			networkInterface: nic,
			libvirt: &libvirtNetworkInterface{
				iface: &iface,
			},
		}
	}
	return res, nil
}

func domainDescHostDevices(domainDesc *libvirtxml.Domain) []libvirtxml.DomainHostdev {
	if domainDesc.Devices == nil {
		return nil
	}
	return domainDesc.Devices.Hostdevs
}

func domainDescInterfaces(domainDesc *libvirtxml.Domain) []libvirtxml.DomainInterface {
	if domainDesc.Devices == nil {
		return nil
	}
	return domainDesc.Devices.Interfaces
}

func (r *MachineReconciler) desiredNetworkInterfaces(machine *api.Machine) map[string]*api.NetworkInterfaceSpec {
	res := make(map[string]*api.NetworkInterfaceSpec)
	for _, nic := range machine.Spec.NetworkInterfaces {
		res[nic.Name] = nic
	}
	return res
}

func (r *MachineReconciler) attachDomainDevice(domain libvirt.Domain, dev libvirtxml.Document) error {
	data, err := dev.Marshal()
	if err != nil {
		return err
	}
	return r.libvirt.DomainAttachDevice(domain, data)
}

func (r *MachineReconciler) detachDomainDevice(domain libvirt.Domain, dev libvirtxml.Document) error {
	data, err := dev.Marshal()
	if err != nil {
		return err
	}
	return r.libvirt.DomainDetachDevice(domain, data)
}

func parseNetworkInterfaceAlias(alias string) (string, error) {
	if !strings.HasPrefix(alias, networkInterfaceAliasPrefix) {
		return "", errNoNetworkInterfaceAlias
	}
	return strings.TrimPrefix(alias, networkInterfaceAliasPrefix), nil
}

func libvirtHostdevToVirtletNetworkInterface(hostDev *libvirtxml.DomainHostdev) (*virtletnetworkinterface.NetworkInterface, error) {
	if hostDev.Managed != "yes" {
		return &virtletnetworkinterface.NetworkInterface{}, fmt.Errorf("non-managed host device: %#v", hostDev)
	}
	if hostDev.SubsysPCI == nil || hostDev.SubsysPCI.Source == nil || hostDev.SubsysPCI.Source.Address == nil {
		return nil, fmt.Errorf("no pci subsystem: %#v", hostDev)
	}
	if hostDev.Address == nil || hostDev.Address.PCI == nil {
		return nil, fmt.Errorf("no pci address: %#v", hostDev)
	}

	sourceAddr := hostDev.SubsysPCI.Source.Address

	if sourceAddr.Domain == nil || sourceAddr.Bus == nil || sourceAddr.Slot == nil || sourceAddr.Function == nil {
		return nil, fmt.Errorf("missing pci subsystem source address fields: %#v", sourceAddr)
	}

	return &virtletnetworkinterface.NetworkInterface{
		HostDevice: &virtletnetworkinterface.HostDevice{
			Domain:   *sourceAddr.Domain,
			Bus:      *sourceAddr.Bus,
			Slot:     *sourceAddr.Slot,
			Function: *sourceAddr.Function,
		},
	}, nil
}

func libvirtInterfaceToVirtletNetworkInterface(iface *libvirtxml.DomainInterface) (*virtletnetworkinterface.NetworkInterface, error) {
	src := iface.Source
	if src == nil {
		return nil, fmt.Errorf("no interface source specified")
	}

	switch {
	case src.User != nil:
		return &virtletnetworkinterface.NetworkInterface{
			Isolated: &virtletnetworkinterface.Isolated{},
		}, nil
	case src.Network != nil:
		return &virtletnetworkinterface.NetworkInterface{
			ProviderNetwork: &virtletnetworkinterface.ProviderNetwork{
				NetworkName: src.Network.Network,
			},
		}, nil
	default:
		return nil, fmt.Errorf("invalid network source")
	}
}

func networkInterfaceAlias(name string) string {
	return fmt.Sprintf("%s%s", networkInterfaceAliasPrefix, name)
}

func virtletNetworkInterfaceToLibvirt(name string, nic *virtletnetworkinterface.NetworkInterface) (*libvirtNetworkInterface, error) {
	switch {
	case nic.HostDevice != nil:
		var zero uint
		return &libvirtNetworkInterface{
			hostDev: &libvirtxml.DomainHostdev{
				Alias: &libvirtxml.DomainAlias{
					Name: networkInterfaceAlias(name),
				},
				Managed: "yes",
				SubsysPCI: &libvirtxml.DomainHostdevSubsysPCI{
					Source: &libvirtxml.DomainHostdevSubsysPCISource{
						Address: &libvirtxml.DomainAddressPCI{
							Domain:   &nic.HostDevice.Domain,
							Bus:      &nic.HostDevice.Bus,
							Slot:     &nic.HostDevice.Slot,
							Function: &nic.HostDevice.Function,
						},
					},
				},
				Address: &libvirtxml.DomainAddress{
					PCI: &libvirtxml.DomainAddressPCI{
						Domain:   &nic.HostDevice.Domain,
						Bus:      &nic.HostDevice.Bus,
						Slot:     &zero,
						Function: &zero,
					},
				},
			},
		}, nil
	case nic.Isolated != nil:
		return &libvirtNetworkInterface{
			iface: &libvirtxml.DomainInterface{
				Alias: &libvirtxml.DomainAlias{
					Name: networkInterfaceAlias(name),
				},
				Source: &libvirtxml.DomainInterfaceSource{
					User: &libvirtxml.DomainInterfaceSourceUser{},
				},
			},
		}, nil
	case nic.ProviderNetwork != nil:
		return &libvirtNetworkInterface{
			iface: &libvirtxml.DomainInterface{
				Alias: &libvirtxml.DomainAlias{
					Name: networkInterfaceAlias(name),
				},
				Source: &libvirtxml.DomainInterfaceSource{
					Network: &libvirtxml.DomainInterfaceSourceNetwork{
						Network: nic.ProviderNetwork.NetworkName,
					},
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported virtlet network interface: %#+v", nic)
	}
}
