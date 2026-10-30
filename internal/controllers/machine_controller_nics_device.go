// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/isolated"
	"github.com/ironcore-dev/provider-utils/plugin"
	"libvirt.org/go/libvirtxml"
)

var (
	errNetworkInterfaceNotReady = errors.New("network interface not ready")
	errNetworkInterfaceFailed   = errors.New("network interface plugin reported an error")
)

// nicDevice is the device a network interface plugin prepared for the guest.
// Exactly one member is set.
type nicDevice struct {
	PCI    *plugin.PCI
	TAP    *plugin.TAP
	Netdev *plugin.Netdev
}

// networkInterfaceSpecToPlugin converts the machine's network interface spec
// into the spec handed to the network interface plugin.
func networkInterfaceSpecToPlugin(nic *api.NetworkInterfaceSpec, machine *api.Machine) *plugin.NetworkInterfaceSpec {
	spec := &plugin.NetworkInterfaceSpec{
		Name:      nic.Name,
		NetworkId: nic.NetworkId,
		Ips:       append([]string(nil), nic.Ips...),
	}
	if nic.Metadata != nil {
		spec.Metadata = &plugin.NetworkInterfaceMetadata{
			ID:        nic.Metadata.ID,
			Name:      nic.Metadata.Name,
			Namespace: nic.Metadata.Namespace,
		}
	}
	if machine.Spec.GuestConfig != nil {
		spec.Hostname = machine.Spec.GuestConfig.HostName
	}
	return spec
}

// nicDeviceFromStatus extracts the single device of a ready status. PCI
// addresses are normalized to their canonical form.
func nicDeviceFromStatus(status *plugin.NetworkInterfaceStatus) (*nicDevice, error) {
	var count int
	for _, set := range []bool{status.PCI != nil, status.TAP != nil, status.Netdev != nil} {
		if set {
			count++
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("ready network interface must have exactly one device, got %d", count)
	}

	switch {
	case status.PCI != nil:
		return &nicDevice{PCI: &plugin.PCI{BDF: status.PCI.BDF}}, nil
	case status.TAP != nil:
		if status.TAP.Name == "" {
			return nil, fmt.Errorf("TAP device without name")
		}
		return &nicDevice{TAP: &plugin.TAP{Name: status.TAP.Name}}, nil
	default:
		if status.Netdev.Name == "" {
			return nil, fmt.Errorf("netdev without name")
		}
		return &nicDevice{Netdev: &plugin.Netdev{Name: status.Netdev.Name}}, nil
	}
}

var networkInterfacePluginToProviderState = map[plugin.NetworkInterfaceState]api.NetworkInterfaceState{
	plugin.NetworkInterfaceStatePending: api.NetworkInterfaceStatePending,
	plugin.NetworkInterfaceStateReady:   api.NetworkInterfaceStateReady,
	plugin.NetworkInterfaceStateError:   api.NetworkInterfaceStateError,
}

func networkInterfaceStatus(name string, status *plugin.NetworkInterfaceStatus) api.NetworkInterfaceStatus {
	res := api.NetworkInterfaceStatus{
		Name:   name,
		State:  networkInterfacePluginToProviderState[status.State],
		Handle: status.Handle,
	}
	for _, ip := range status.Ips {
		res.Ips = append(res.Ips, ip.String())
	}
	for _, prefix := range status.Prefixes {
		res.Prefixes = append(res.Prefixes, prefix.String())
	}
	if status.VirtualIp.IsValid() {
		res.VirtualIP = status.VirtualIp.String()
	}
	return res
}

// pciAddress is a parsed PCI function address.
type pciAddress struct {
	Domain, Bus, Slot, Function uint
}

// String formats the address in canonical form `dddd:bb:ss.f`.
func (a pciAddress) String() string {
	return fmt.Sprintf("%04x:%02x:%02x.%x", a.Domain, a.Bus, a.Slot, a.Function)
}

// parseBDF parses a PCI address in the format `(domain:)bus:device.function`
// with all components in hexadecimal. A missing domain defaults to 0.
func parseBDF(bdf string) (pciAddress, error) {
	devFn, funcStr, ok := strings.Cut(bdf, ".")
	if !ok {
		return pciAddress{}, fmt.Errorf("invalid PCI address %q: missing function", bdf)
	}

	parts := strings.Split(devFn, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return pciAddress{}, fmt.Errorf("invalid PCI address %q: expected format (domain:)bus:device.function", bdf)
	}
	parts = append(parts, funcStr)

	// domain is optional
	if len(parts) == 3 {
		parts = append([]string{"0"}, parts...)
	}

	limits := []struct {
		name string
		max  uint64
	}{{"domain", 0xffff}, {"bus", 0xff}, {"device", 0x1f}, {"function", 0x7}}

	var values [4]uint
	for i, part := range parts {
		v, err := strconv.ParseUint(part, 16, 32)
		if err != nil {
			return pciAddress{}, fmt.Errorf("invalid PCI address %q: error parsing %s %q: %w", bdf, limits[i].name, part, err)
		}
		if v > limits[i].max {
			return pciAddress{}, fmt.Errorf("invalid PCI address %q: %s %#x out of range", bdf, limits[i].name, v)
		}
		values[i] = uint(v)
	}
	return pciAddress{Domain: values[0], Bus: values[1], Slot: values[2], Function: values[3]}, nil
}

// nicDeviceToLibvirt renders the device as libvirt domain device.
func nicDeviceToLibvirt(name string, dev *nicDevice) (*libvirtNetworkInterface, error) {
	alias := &libvirtxml.DomainAlias{Name: networkInterfaceAlias(name)}

	switch {
	case dev.PCI != nil:
		addr, err := parseBDF(dev.PCI.BDF)
		if err != nil {
			return nil, err
		}
		return &libvirtNetworkInterface{
			hostDev: &libvirtxml.DomainHostdev{
				Alias:   alias,
				Managed: "yes",
				SubsysPCI: &libvirtxml.DomainHostdevSubsysPCI{
					Source: &libvirtxml.DomainHostdevSubsysPCISource{
						Address: &libvirtxml.DomainAddressPCI{
							Domain:   &addr.Domain,
							Bus:      &addr.Bus,
							Slot:     &addr.Slot,
							Function: &addr.Function,
						},
					},
				},
				Address: &libvirtxml.DomainAddress{
					// if not defined, a non-conflicting pci address will be selected
					PCI: &libvirtxml.DomainAddressPCI{},
				},
			},
		}, nil
	case dev.Netdev != nil && dev.Netdev.Name == isolated.NetdevName:
		// TODO: remove once inisix lands. A real netdev named like the isolated
		// plugin's netdev is wrongly turned into user-mode networking.
		return &libvirtNetworkInterface{
			iface: &libvirtxml.DomainInterface{
				Alias: alias,
				Source: &libvirtxml.DomainInterfaceSource{
					User: &libvirtxml.DomainInterfaceSourceUser{},
				},
			},
		}, nil
	case dev.Netdev != nil:
		return &libvirtNetworkInterface{
			iface: &libvirtxml.DomainInterface{
				Alias: alias,
				Model: &libvirtxml.DomainInterfaceModel{Type: "virtio"},
				Source: &libvirtxml.DomainInterfaceSource{
					Direct: &libvirtxml.DomainInterfaceSourceDirect{
						Dev:  dev.Netdev.Name,
						Mode: "bridge",
					},
				},
			},
		}, nil
	case dev.TAP != nil:
		return &libvirtNetworkInterface{
			iface: &libvirtxml.DomainInterface{
				Alias: alias,
				Model: &libvirtxml.DomainInterfaceModel{Type: "virtio"},
				Source: &libvirtxml.DomainInterfaceSource{
					Ethernet: &libvirtxml.DomainInterfaceSourceEthernet{},
				},
				Target: &libvirtxml.DomainInterfaceTarget{
					Dev:     dev.TAP.Name,
					Managed: "no",
				},
				Driver: &libvirtxml.DomainInterfaceDriver{
					Name:   "vhost",
					Queues: 2,
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("network interface %q without device", name)
	}
}

// libvirtHostdevToNicDevice converts a libvirt hostdev back to the device.
func libvirtHostdevToNicDevice(hostDev *libvirtxml.DomainHostdev) (*nicDevice, error) {
	if hostDev.Managed != "yes" {
		return nil, fmt.Errorf("non-managed host device: %#v", hostDev)
	}
	if hostDev.SubsysPCI == nil || hostDev.SubsysPCI.Source == nil || hostDev.SubsysPCI.Source.Address == nil {
		return nil, fmt.Errorf("no pci subsystem: %#v", hostDev)
	}
	if hostDev.Address == nil || hostDev.Address.PCI == nil {
		return nil, fmt.Errorf("no pci address: %#v", hostDev)
	}

	src := hostDev.SubsysPCI.Source.Address
	if src.Domain == nil || src.Bus == nil || src.Slot == nil || src.Function == nil {
		return nil, fmt.Errorf("missing pci subsystem source address fields: %#v", src)
	}

	addr := pciAddress{Domain: *src.Domain, Bus: *src.Bus, Slot: *src.Slot, Function: *src.Function}
	return &nicDevice{PCI: &plugin.PCI{BDF: addr.String()}}, nil
}

// libvirtInterfaceToNicDevice converts a libvirt interface back to the device.
func libvirtInterfaceToNicDevice(iface *libvirtxml.DomainInterface) (*nicDevice, error) {
	src := iface.Source
	if src == nil {
		return nil, fmt.Errorf("no interface source specified")
	}

	switch {
	case src.User != nil:
		return &nicDevice{Netdev: &plugin.Netdev{Name: isolated.NetdevName}}, nil
	case src.Direct != nil:
		return &nicDevice{Netdev: &plugin.Netdev{Name: src.Direct.Dev}}, nil
	case src.Ethernet != nil:
		if iface.Target == nil || iface.Target.Dev == "" {
			return nil, fmt.Errorf("ethernet interface without target device: %#v", iface)
		}
		return &nicDevice{TAP: &plugin.TAP{Name: iface.Target.Dev}}, nil
	default:
		return nil, fmt.Errorf("invalid network source")
	}
}
