// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

// Internal test, plain stdlib: the network interface conversions are unexported, and a ginkgo
// spec in this directory would be collected by the libvirt-backed suite in package controllers_test.
package controllers

import (
	"encoding/xml"
	"reflect"
	"testing"

	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	"libvirt.org/go/libvirtxml"
)

func TestProviderNetworkInterfaceToLibvirtRoundTrip(t *testing.T) {
	tests := map[string]*providernetworkinterface.NetworkInterface{
		"host device": {
			HostDevice: &providernetworkinterface.HostDevice{
				Domain:   0,
				Bus:      0x1f,
				Slot:     0,
				Function: 1,
			},
		},
		"direct": {
			Direct: &providernetworkinterface.Direct{
				Dev: "net_tap0",
			},
		},
		"ethernet": {
			Ethernet: &providernetworkinterface.Ethernet{
				Dev: "tap-0123456789ab",
			},
		},
		"isolated": {
			Isolated: &providernetworkinterface.Isolated{},
		},
		"provider network": {
			ProviderNetwork: &providernetworkinterface.ProviderNetwork{
				NetworkName: "provider-net",
			},
		},
	}

	for name, providerNic := range tests {
		t.Run(name, func(t *testing.T) {
			libvirtNic, err := providerNetworkInterfaceToLibvirt("nic0", providerNic)
			if err != nil {
				t.Fatalf("error converting provider network interface to libvirt: %v", err)
			}

			data, err := libvirtNic.device().Marshal()
			if err != nil {
				t.Fatalf("error marshalling libvirt device: %v", err)
			}

			var readBack *providernetworkinterface.NetworkInterface
			switch doc := libvirtNic.device().(type) {
			case *libvirtxml.DomainHostdev:
				hostDev := &libvirtxml.DomainHostdev{}
				if err := xml.Unmarshal([]byte(data), hostDev); err != nil {
					t.Fatalf("error unmarshalling hostdev: %v", err)
				}
				readBack, err = libvirtHostdevToProviderNetworkInterface(hostDev)
			case *libvirtxml.DomainInterface:
				iface := &libvirtxml.DomainInterface{}
				if err := xml.Unmarshal([]byte(data), iface); err != nil {
					t.Fatalf("error unmarshalling interface: %v", err)
				}
				readBack, err = libvirtInterfaceToProviderNetworkInterface(iface)
			default:
				t.Fatalf("unexpected libvirt device type %T", doc)
			}
			if err != nil {
				t.Fatalf("error converting libvirt network interface to provider network interface: %v", err)
			}

			if !reflect.DeepEqual(readBack, providerNic) {
				t.Errorf("round trip mismatch:\n got: %#v\nwant: %#v", readBack, providerNic)
			}
		})
	}
}

func TestProviderNetworkInterfaceToLibvirtEthernetXML(t *testing.T) {
	libvirtNic, err := providerNetworkInterfaceToLibvirt("nic0", &providernetworkinterface.NetworkInterface{
		Ethernet: &providernetworkinterface.Ethernet{
			Dev: "tap-mine",
		},
	})
	if err != nil {
		t.Fatalf("error converting provider network interface to libvirt: %v", err)
	}

	data, err := libvirtNic.device().Marshal()
	if err != nil {
		t.Fatalf("error marshalling libvirt device: %v", err)
	}

	want := `<interface type="ethernet">
  <target dev="tap-mine" managed="no"></target>
  <model type="virtio"></model>
  <driver name="vhost" queues="2"></driver>
  <alias name="ua-networkinterface-nic0"></alias>
</interface>`
	if string(data) != want {
		t.Errorf("unexpected domain interface XML:\n got: %s\nwant: %s", data, want)
	}
}

func TestLibvirtInterfaceToProviderNetworkInterfaceEthernetWithoutTarget(t *testing.T) {
	iface := &libvirtxml.DomainInterface{
		Source: &libvirtxml.DomainInterfaceSource{
			Ethernet: &libvirtxml.DomainInterfaceSourceEthernet{},
		},
	}
	if _, err := libvirtInterfaceToProviderNetworkInterface(iface); err == nil {
		t.Error("expected error for ethernet interface without target device, got nil")
	}
}

func TestAlignReportedNetworkInterfaceFields(t *testing.T) {
	applied := &providernetworkinterface.NetworkInterface{
		Handle:   "attachment-1",
		Prefixes: []string{"2001:db8:4000:100::/64"},
		Ethernet: &providernetworkinterface.Ethernet{Dev: "inisix-tap0"},
	}

	libvirtNic, err := providerNetworkInterfaceToLibvirt("nic0", applied)
	if err != nil {
		t.Fatalf("error converting provider network interface to libvirt: %v", err)
	}
	data, err := libvirtNic.device().Marshal()
	if err != nil {
		t.Fatalf("error marshalling libvirt device: %v", err)
	}
	iface := &libvirtxml.DomainInterface{}
	if err := xml.Unmarshal([]byte(data), iface); err != nil {
		t.Fatalf("error unmarshalling interface: %v", err)
	}

	readBack, err := libvirtInterfaceToProviderNetworkInterface(iface)
	if err != nil {
		t.Fatalf("error reading back provider network interface: %v", err)
	}
	if reflect.DeepEqual(readBack, applied) {
		t.Error("expected read-back interface to differ from applied interface before alignment")
	}

	alignReportedNetworkInterfaceFields(readBack, applied)
	if !reflect.DeepEqual(readBack, applied) {
		t.Errorf("expected interfaces to compare equal after alignment, readBack: %#v, applied: %#v", readBack, applied)
	}

	alignedHandle := applied.Handle
	applied.Handle = "attachment-2"
	applied.Prefixes = []string{"2001:db8:4000:200::/64"}
	if alignedHandle == applied.Handle {
		t.Fatal("test setup: handle should have changed")
	}
	alignReportedNetworkInterfaceFields(readBack, applied)
	if !reflect.DeepEqual(readBack, applied) {
		t.Errorf("expected changed report-only values to compare equal after alignment, readBack: %#v, applied: %#v", readBack, applied)
	}
}
