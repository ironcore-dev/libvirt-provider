// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/isolated"
	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"
	"github.com/ironcore-dev/provider-utils/plugin"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"
)

func TestParseBDF(t *testing.T) {
	tests := []struct {
		bdf     string
		want    pciAddress
		wantErr bool
	}{
		{bdf: "0000:3b:00.1", want: pciAddress{Domain: 0, Bus: 0x3b, Slot: 0, Function: 1}},
		{bdf: "3b:00.1", want: pciAddress{Domain: 0, Bus: 0x3b, Slot: 0, Function: 1}},
		{bdf: "0001:af:1f.7", want: pciAddress{Domain: 1, Bus: 0xaf, Slot: 0x1f, Function: 7}},
		{bdf: "0000:3B:0A.0", want: pciAddress{Domain: 0, Bus: 0x3b, Slot: 0x0a, Function: 0}},
		{bdf: "", wantErr: true},
		{bdf: "0000:3b:00", wantErr: true},
		{bdf: "3b.1", wantErr: true},
		{bdf: "1:2:3:4.1", wantErr: true},
		{bdf: "0000:3b:00.8", wantErr: true},
		{bdf: "0000:3b:20.0", wantErr: true},
		{bdf: "0000:100:00.0", wantErr: true},
		{bdf: "10000:00:00.0", wantErr: true},
		{bdf: "0000:zz:00.0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.bdf, func(t *testing.T) {
			g := NewWithT(t)
			got, err := parseBDF(tt.bdf)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestPCIAddressString(t *testing.T) {
	g := NewWithT(t)
	g.Expect(pciAddress{Domain: 0, Bus: 0x3b, Slot: 0, Function: 1}.String()).To(Equal("0000:3b:00.1"))
}

func TestNicDeviceToLibvirt(t *testing.T) {
	t.Run("pci", func(t *testing.T) {
		g := NewWithT(t)
		nic, err := nicDeviceToLibvirt("nic", &nicDevice{PCI: &plugin.PCI{BDF: "0000:3b:00.1"}})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(nic.iface).To(BeNil())
		g.Expect(nic.hostDev.Managed).To(Equal("yes"))
		g.Expect(nic.hostDev.Alias.Name).To(Equal(networkInterfaceAlias("nic")))
		src := nic.hostDev.SubsysPCI.Source.Address
		g.Expect(*src.Domain).To(BeEquivalentTo(0))
		g.Expect(*src.Bus).To(BeEquivalentTo(0x3b))
		g.Expect(*src.Slot).To(BeEquivalentTo(0))
		g.Expect(*src.Function).To(BeEquivalentTo(1))
	})

	t.Run("invalid pci", func(t *testing.T) {
		g := NewWithT(t)
		_, err := nicDeviceToLibvirt("nic", &nicDevice{PCI: &plugin.PCI{BDF: "bogus"}})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("netdev", func(t *testing.T) {
		g := NewWithT(t)
		nic, err := nicDeviceToLibvirt("nic", &nicDevice{Netdev: &plugin.Netdev{Name: "eth1"}})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(nic.hostDev).To(BeNil())
		g.Expect(nic.iface.Model.Type).To(Equal("virtio"))
		g.Expect(nic.iface.Source.Direct).To(Equal(&libvirtxml.DomainInterfaceSourceDirect{Dev: "eth1", Mode: "bridge"}))
	})

	t.Run("tap", func(t *testing.T) {
		g := NewWithT(t)
		nic, err := nicDeviceToLibvirt("nic", &nicDevice{TAP: &plugin.TAP{Name: "tap0"}})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(nic.iface.Model.Type).To(Equal("virtio"))
		g.Expect(nic.iface.Source.Ethernet).NotTo(BeNil())
		g.Expect(nic.iface.Target).To(Equal(&libvirtxml.DomainInterfaceTarget{Dev: "tap0", Managed: "no"}))
		g.Expect(nic.iface.Driver.Name).To(Equal("vhost"))
	})

	t.Run("isolated", func(t *testing.T) {
		g := NewWithT(t)
		nic, err := nicDeviceToLibvirt("nic", &nicDevice{Netdev: &plugin.Netdev{Name: isolated.NetdevName}})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(nic.iface.Source.User).NotTo(BeNil())
		g.Expect(nic.iface.Source.Direct).To(BeNil())
	})

	t.Run("no device", func(t *testing.T) {
		g := NewWithT(t)
		_, err := nicDeviceToLibvirt("nic", &nicDevice{})
		g.Expect(err).To(HaveOccurred())
	})
}

func TestNicDeviceLibvirtRoundTrip(t *testing.T) {
	devices := map[string]*nicDevice{
		"pci":      {PCI: &plugin.PCI{BDF: "0000:3b:00.1"}},
		"netdev":   {Netdev: &plugin.Netdev{Name: "eth1"}},
		"tap":      {TAP: &plugin.TAP{Name: "tap0"}},
		"isolated": {Netdev: &plugin.Netdev{Name: isolated.NetdevName}},
	}
	for name, dev := range devices {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)

			nic, err := nicDeviceToLibvirt("nic", dev)
			g.Expect(err).NotTo(HaveOccurred())

			// go through XML to mimic reading the domain back from libvirt
			var got *nicDevice
			switch {
			case nic.hostDev != nil:
				data, err := nic.hostDev.Marshal()
				g.Expect(err).NotTo(HaveOccurred())
				hostDev := &libvirtxml.DomainHostdev{}
				g.Expect(hostDev.Unmarshal(data)).To(Succeed())
				got, err = libvirtHostdevToNicDevice(hostDev)
				g.Expect(err).NotTo(HaveOccurred())
			default:
				data, err := nic.iface.Marshal()
				g.Expect(err).NotTo(HaveOccurred())
				iface := &libvirtxml.DomainInterface{}
				g.Expect(iface.Unmarshal(data)).To(Succeed())
				got, err = libvirtInterfaceToNicDevice(iface)
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(got).To(Equal(dev))
		})
	}
}

func TestLibvirtReadBackErrors(t *testing.T) {
	g := NewWithT(t)

	_, err := libvirtHostdevToNicDevice(&libvirtxml.DomainHostdev{Managed: "no"})
	g.Expect(err).To(HaveOccurred())

	_, err = libvirtInterfaceToNicDevice(&libvirtxml.DomainInterface{})
	g.Expect(err).To(HaveOccurred())

	_, err = libvirtInterfaceToNicDevice(&libvirtxml.DomainInterface{
		Source: &libvirtxml.DomainInterfaceSource{Ethernet: &libvirtxml.DomainInterfaceSourceEthernet{}},
	})
	g.Expect(err).To(HaveOccurred())

	_, err = libvirtInterfaceToNicDevice(&libvirtxml.DomainInterface{
		Source: &libvirtxml.DomainInterfaceSource{Network: &libvirtxml.DomainInterfaceSourceNetwork{Network: "default"}},
	})
	g.Expect(err).To(HaveOccurred())
}

func TestNetworkInterfaceSpecToPlugin(t *testing.T) {
	g := NewWithT(t)

	machine := &api.Machine{Spec: api.MachineSpec{GuestConfig: &api.MachineGuestConfig{HostName: "host"}}}
	nic := &api.NetworkInterfaceSpec{
		Metadata:   &api.NetworkInterfaceMetadata{ID: "id", Name: "name", Namespace: "ns"},
		Name:       "nic",
		NetworkId:  "net",
		Ips:        []string{"10.0.0.1"},
		Attributes: map[string]string{"foo": "bar"},
	}

	g.Expect(networkInterfaceSpecToPlugin(nic, machine)).To(Equal(&plugin.NetworkInterfaceSpec{
		Metadata:  &plugin.NetworkInterfaceMetadata{ID: "id", Name: "name", Namespace: "ns"},
		Name:      "nic",
		NetworkId: "net",
		Hostname:  "host",
		Ips:       []string{"10.0.0.1"},
	}))

	g.Expect(networkInterfaceSpecToPlugin(&api.NetworkInterfaceSpec{Name: "nic"}, &api.Machine{})).To(Equal(&plugin.NetworkInterfaceSpec{
		Name: "nic",
	}))
}

func TestNicDeviceFromStatus(t *testing.T) {
	t.Run("pci", func(t *testing.T) {
		g := NewWithT(t)
		dev, err := nicDeviceFromStatus(&plugin.NetworkInterfaceStatus{PCI: &plugin.PCI{BDF: "0000:3b:00.1"}})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(dev).To(Equal(&nicDevice{PCI: &plugin.PCI{BDF: "0000:3b:00.1"}}))
	})

	t.Run("tap", func(t *testing.T) {
		g := NewWithT(t)
		dev, err := nicDeviceFromStatus(&plugin.NetworkInterfaceStatus{TAP: &plugin.TAP{Name: "tap0"}})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(dev).To(Equal(&nicDevice{TAP: &plugin.TAP{Name: "tap0"}}))
	})

	t.Run("netdev", func(t *testing.T) {
		g := NewWithT(t)
		dev, err := nicDeviceFromStatus(&plugin.NetworkInterfaceStatus{Netdev: &plugin.Netdev{Name: "eth1"}})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(dev).To(Equal(&nicDevice{Netdev: &plugin.Netdev{Name: "eth1"}}))
	})

	t.Run("no device", func(t *testing.T) {
		g := NewWithT(t)
		_, err := nicDeviceFromStatus(&plugin.NetworkInterfaceStatus{})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("multiple devices", func(t *testing.T) {
		g := NewWithT(t)
		_, err := nicDeviceFromStatus(&plugin.NetworkInterfaceStatus{
			PCI:    &plugin.PCI{BDF: "0000:3b:00.1"},
			Netdev: &plugin.Netdev{Name: "eth1"},
		})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("tap without name", func(t *testing.T) {
		g := NewWithT(t)
		_, err := nicDeviceFromStatus(&plugin.NetworkInterfaceStatus{TAP: &plugin.TAP{}})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("netdev without name", func(t *testing.T) {
		g := NewWithT(t)
		_, err := nicDeviceFromStatus(&plugin.NetworkInterfaceStatus{Netdev: &plugin.Netdev{}})
		g.Expect(err).To(HaveOccurred())
	})
}

func TestNetworkInterfaceStatus(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(networkInterfaceStatus("nic", &plugin.NetworkInterfaceStatus{
			Handle:    "handle",
			State:     plugin.NetworkInterfaceStateReady,
			Ips:       []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			Prefixes:  []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
			VirtualIp: netip.MustParseAddr("1.2.3.4"),
		})).To(Equal(api.NetworkInterfaceStatus{
			Name:      "nic",
			Handle:    "handle",
			State:     api.NetworkInterfaceStateReady,
			Ips:       []string{"10.0.0.1"},
			Prefixes:  []string{"10.0.0.0/24"},
			VirtualIP: "1.2.3.4",
		}))
	})

	t.Run("pending", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(networkInterfaceStatus("nic", &plugin.NetworkInterfaceStatus{
			Handle: "handle",
			State:  plugin.NetworkInterfaceStatePending,
		})).To(Equal(api.NetworkInterfaceStatus{
			Name:   "nic",
			Handle: "handle",
			State:  api.NetworkInterfaceStatePending,
		}))
	})

	t.Run("error", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(networkInterfaceStatus("nic", &plugin.NetworkInterfaceStatus{
			Handle: "handle",
			State:  plugin.NetworkInterfaceStateError,
		})).To(Equal(api.NetworkInterfaceStatus{
			Name:   "nic",
			Handle: "handle",
			State:  api.NetworkInterfaceStateError,
		}))
	})
}

type fakeNicPlugin struct {
	plugin.NetworkInterface

	status   *plugin.NetworkInterfaceStatus
	applyErr error

	deleted []string
	failFor map[string]bool
}

func (f *fakeNicPlugin) Apply(_ context.Context, _ *plugin.NetworkInterfaceSpec, _ string) (*plugin.NetworkInterfaceStatus, error) {
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	return f.status, nil
}

func (f *fakeNicPlugin) Delete(_ context.Context, spec *plugin.NetworkInterfaceSpec, _ string) error {
	if f.failFor[spec.Name] {
		return errors.New("delete failed")
	}
	f.deleted = append(f.deleted, spec.Name)
	return nil
}

func TestApplyNetworkInterface(t *testing.T) {
	machine := &api.Machine{Metadata: apiutils.Metadata{ID: "machine"}}
	nic := &api.NetworkInterfaceSpec{Name: "nic"}

	t.Run("ready", func(t *testing.T) {
		g := NewWithT(t)
		r := &MachineReconciler{networkInterfacePlugin: &fakeNicPlugin{status: &plugin.NetworkInterfaceStatus{
			Handle: "handle",
			State:  plugin.NetworkInterfaceStateReady,
			PCI:    &plugin.PCI{BDF: "0000:3b:00.1"},
		}}}

		status, dev, err := r.applyNetworkInterface(context.Background(), machine, nic)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(status).To(Equal(&api.NetworkInterfaceStatus{
			Name:   "nic",
			Handle: "handle",
			State:  api.NetworkInterfaceStateReady,
		}))
		g.Expect(dev).To(Equal(&nicDevice{PCI: &plugin.PCI{BDF: "0000:3b:00.1"}}))
	})

	t.Run("pending", func(t *testing.T) {
		g := NewWithT(t)
		r := &MachineReconciler{networkInterfacePlugin: &fakeNicPlugin{status: &plugin.NetworkInterfaceStatus{
			Handle: "handle",
			State:  plugin.NetworkInterfaceStatePending,
		}}}

		status, dev, err := r.applyNetworkInterface(context.Background(), machine, nic)
		g.Expect(err).To(MatchError(errNetworkInterfaceNotReady))
		g.Expect(status).To(Equal(&api.NetworkInterfaceStatus{
			Name:   "nic",
			Handle: "handle",
			State:  api.NetworkInterfaceStatePending,
		}))
		g.Expect(dev).To(BeNil())
	})

	t.Run("error", func(t *testing.T) {
		g := NewWithT(t)
		r := &MachineReconciler{networkInterfacePlugin: &fakeNicPlugin{status: &plugin.NetworkInterfaceStatus{
			Handle: "handle",
			State:  plugin.NetworkInterfaceStateError,
		}}}

		status, dev, err := r.applyNetworkInterface(context.Background(), machine, nic)
		g.Expect(err).To(MatchError(errNetworkInterfaceFailed))
		g.Expect(status).To(Equal(&api.NetworkInterfaceStatus{
			Name:   "nic",
			Handle: "handle",
			State:  api.NetworkInterfaceStateError,
		}))
		g.Expect(dev).To(BeNil())
	})

	t.Run("ready without device", func(t *testing.T) {
		g := NewWithT(t)
		r := &MachineReconciler{networkInterfacePlugin: &fakeNicPlugin{status: &plugin.NetworkInterfaceStatus{
			Handle: "handle",
			State:  plugin.NetworkInterfaceStateReady,
		}}}

		status, dev, err := r.applyNetworkInterface(context.Background(), machine, nic)
		g.Expect(err).To(HaveOccurred())
		g.Expect(status).To(Equal(&api.NetworkInterfaceStatus{
			Name:   "nic",
			Handle: "handle",
			State:  api.NetworkInterfaceStateError,
		}))
		g.Expect(dev).To(BeNil())
	})

	t.Run("plugin error", func(t *testing.T) {
		g := NewWithT(t)
		applyErr := errors.New("apply failed")
		r := &MachineReconciler{networkInterfacePlugin: &fakeNicPlugin{applyErr: applyErr}}

		status, dev, err := r.applyNetworkInterface(context.Background(), machine, nic)
		g.Expect(err).To(MatchError(applyErr))
		g.Expect(status).To(BeNil())
		g.Expect(dev).To(BeNil())
	})
}

func TestDeleteNetworkInterfaces(t *testing.T) {
	deletionTimestamp := time.Now()
	newMachine := func() *api.Machine {
		return &api.Machine{
			Spec: api.MachineSpec{NetworkInterfaces: []*api.NetworkInterfaceSpec{
				{Name: "kept"},
				{Name: "gone", DeletionTimestamp: &deletionTimestamp},
				{Name: "nostatus", DeletionTimestamp: &deletionTimestamp},
				{Name: "broken", DeletionTimestamp: &deletionTimestamp},
				{Name: "mounted", DeletionTimestamp: &deletionTimestamp},
			}},
			Status: api.MachineStatus{NetworkInterfaceStatus: []api.NetworkInterfaceStatus{
				{Name: "kept", Handle: "h-kept", State: api.NetworkInterfaceStateAttached},
				{Name: "gone", Handle: "h-gone", State: api.NetworkInterfaceStateReady},
				{Name: "broken", Handle: "h-broken", State: api.NetworkInterfaceStateReady},
				{Name: "mounted", Handle: "h-mounted", State: api.NetworkInterfaceStateAttached},
			}},
		}
	}

	t.Run("deletes unmounted nics marked for deletion", func(t *testing.T) {
		g := NewWithT(t)
		fake := &fakeNicPlugin{failFor: map[string]bool{"broken": true}}
		r := &MachineReconciler{networkInterfacePlugin: fake}
		machine := newMachine()

		errs := r.deleteNetworkInterfaces(context.Background(), logr.Discard(), machine, map[string]mountedNetworkInterface{
			"kept":    {},
			"mounted": {},
		})

		g.Expect(errs).To(HaveLen(1))
		g.Expect(fake.deleted).To(ConsistOf("gone", "nostatus"))
		g.Expect(machine.Spec.NetworkInterfaces).To(Equal([]*api.NetworkInterfaceSpec{
			{Name: "kept"},
			{Name: "broken", DeletionTimestamp: &deletionTimestamp},
			{Name: "mounted", DeletionTimestamp: &deletionTimestamp},
		}))
		g.Expect(machine.Status.NetworkInterfaceStatus).To(Equal([]api.NetworkInterfaceStatus{
			{Name: "kept", Handle: "h-kept", State: api.NetworkInterfaceStateAttached},
			{Name: "broken", Handle: "h-broken", State: api.NetworkInterfaceStateReady},
			{Name: "mounted", Handle: "h-mounted", State: api.NetworkInterfaceStateAttached},
		}))
	})

	t.Run("without mounted nics all nics marked for deletion are deleted", func(t *testing.T) {
		g := NewWithT(t)
		fake := &fakeNicPlugin{}
		r := &MachineReconciler{networkInterfacePlugin: fake}
		machine := newMachine()

		errs := r.deleteNetworkInterfaces(context.Background(), logr.Discard(), machine, nil)

		g.Expect(errs).To(BeEmpty())
		g.Expect(fake.deleted).To(ConsistOf("gone", "nostatus", "broken", "mounted"))
		g.Expect(machine.Spec.NetworkInterfaces).To(Equal([]*api.NetworkInterfaceSpec{
			{Name: "kept"},
		}))
		g.Expect(machine.Status.NetworkInterfaceStatus).To(Equal([]api.NetworkInterfaceStatus{
			{Name: "kept", Handle: "h-kept", State: api.NetworkInterfaceStateAttached},
		}))
	})
}

func TestDeleteAllNetworkInterfaces(t *testing.T) {
	deletionTimestamp := time.Now()
	machine := &api.Machine{
		Spec: api.MachineSpec{NetworkInterfaces: []*api.NetworkInterfaceSpec{
			{Name: "a"},
			{Name: "b"},
			{Name: "c", DeletionTimestamp: &deletionTimestamp},
		}},
		Status: api.MachineStatus{NetworkInterfaceStatus: []api.NetworkInterfaceStatus{
			{Name: "a", Handle: "h-a", State: api.NetworkInterfaceStateAttached},
		}},
	}

	t.Run("deletes all nics of the spec", func(t *testing.T) {
		g := NewWithT(t)
		fake := &fakeNicPlugin{}
		r := &MachineReconciler{networkInterfacePlugin: fake}
		g.Expect(r.deleteAllNetworkInterfaces(context.Background(), logr.Discard(), machine)).To(Succeed())
		g.Expect(fake.deleted).To(ConsistOf("a", "b", "c"))
	})

	t.Run("attempts all and reports failures", func(t *testing.T) {
		g := NewWithT(t)
		fake := &fakeNicPlugin{failFor: map[string]bool{"b": true}}
		r := &MachineReconciler{networkInterfacePlugin: fake}
		g.Expect(r.deleteAllNetworkInterfaces(context.Background(), logr.Discard(), machine)).To(HaveOccurred())
		g.Expect(fake.deleted).To(ConsistOf("a", "c"))
	})
}
