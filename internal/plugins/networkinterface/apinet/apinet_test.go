// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package apinet

import (
	"context"
	"net/netip"
	"testing"

	apinetv1alpha1 "github.com/ironcore-dev/ironcore-net/api/core/v1alpha1"
	apinet "github.com/ironcore-dev/ironcore-net/apimachinery/api/net"
	"github.com/ironcore-dev/ironcore-net/apinetlet/provider"
	"github.com/ironcore-dev/provider-utils/plugin"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testLabel  = "test.example.com/machine-id"
	testHandle = "ironcore-net://ns/nic/node/uid"
)

func newNic(state apinetv1alpha1.NetworkInterfaceState) *apinetv1alpha1.NetworkInterface {
	return &apinetv1alpha1.NetworkInterface{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nic", UID: "uid"},
		Spec: apinetv1alpha1.NetworkInterfaceSpec{
			NodeRef: corev1.LocalObjectReference{Name: "node"},
			IPs:     []apinet.IP{apinet.MustParseIP("10.0.0.1")},
		},
		Status: apinetv1alpha1.NetworkInterfaceStatus{
			State:     state,
			Prefixes:  []apinet.IPPrefix{{Prefix: netip.MustParsePrefix("10.0.0.0/24")}},
			PublicIPs: []apinet.IP{apinet.MustParseIP("1.2.3.4")},
		},
	}
}

func TestNew(t *testing.T) {
	opts := Options{
		NodeName:       "node",
		RestConfig:     &rest.Config{Host: "https://localhost"},
		FieldOwner:     "owner",
		MachineIDLabel: testLabel,
	}

	t.Run("valid options", func(t *testing.T) {
		g := NewWithT(t)
		p, err := New(opts)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(p.Name()).To(Equal("apinet"))
	})

	t.Run("missing node name", func(t *testing.T) {
		g := NewWithT(t)
		o := opts
		o.NodeName = ""
		_, err := New(o)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("missing rest config", func(t *testing.T) {
		g := NewWithT(t)
		o := opts
		o.RestConfig = nil
		_, err := New(o)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("missing field owner", func(t *testing.T) {
		g := NewWithT(t)
		o := opts
		o.FieldOwner = ""
		_, err := New(o)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("missing machine ID label", func(t *testing.T) {
		g := NewWithT(t)
		o := opts
		o.MachineIDLabel = ""
		_, err := New(o)
		g.Expect(err).To(HaveOccurred())
	})
}

func TestStatusFromNic(t *testing.T) {
	t.Run("pending", func(t *testing.T) {
		g := NewWithT(t)
		status, err := statusFromNic(newNic(apinetv1alpha1.NetworkInterfaceStatePending))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(status).To(Equal(&plugin.NetworkInterfaceStatus{Handle: testHandle, State: plugin.NetworkInterfaceStatePending}))
	})

	t.Run("empty state is pending", func(t *testing.T) {
		g := NewWithT(t)
		status, err := statusFromNic(newNic(""))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(status.State).To(Equal(plugin.NetworkInterfaceStatePending))
	})

	t.Run("error", func(t *testing.T) {
		g := NewWithT(t)
		status, err := statusFromNic(newNic(apinetv1alpha1.NetworkInterfaceStateError))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(status).To(Equal(&plugin.NetworkInterfaceStatus{Handle: testHandle, State: plugin.NetworkInterfaceStateError}))
	})

	t.Run("ready with pci", func(t *testing.T) {
		g := NewWithT(t)
		nic := newNic(apinetv1alpha1.NetworkInterfaceStateReady)
		nic.Status.PCIAddress = &apinetv1alpha1.PCIAddress{Domain: "0000", Bus: "3b", Slot: "00", Function: "1"}

		status, err := statusFromNic(nic)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(status).To(Equal(&plugin.NetworkInterfaceStatus{
			Handle:    testHandle,
			State:     plugin.NetworkInterfaceStateReady,
			Prefixes:  []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
			Ips:       []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			VirtualIp: netip.MustParseAddr("1.2.3.4"),
			PCI:       &plugin.PCI{BDF: "0000:3b:00.1"},
		}))
	})

	t.Run("ready with tap", func(t *testing.T) {
		g := NewWithT(t)
		nic := newNic(apinetv1alpha1.NetworkInterfaceStateReady)
		nic.Status.PublicIPs = nil
		nic.Status.TAPDevice = &apinetv1alpha1.TAPDevice{Name: "tap0"}

		status, err := statusFromNic(nic)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(status.State).To(Equal(plugin.NetworkInterfaceStateReady))
		g.Expect(status.Netdev).To(Equal(&plugin.Netdev{Name: "tap0"}))
		g.Expect(status.PCI).To(BeNil())
		g.Expect(status.TAP).To(BeNil())
		g.Expect(status.VirtualIp.IsValid()).To(BeFalse())
	})

	t.Run("ready without device", func(t *testing.T) {
		g := NewWithT(t)
		_, err := statusFromNic(newNic(apinetv1alpha1.NetworkInterfaceStateReady))
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("ready with both devices", func(t *testing.T) {
		g := NewWithT(t)
		nic := newNic(apinetv1alpha1.NetworkInterfaceStateReady)
		nic.Status.PCIAddress = &apinetv1alpha1.PCIAddress{Domain: "0000", Bus: "3b", Slot: "00", Function: "1"}
		nic.Status.TAPDevice = &apinetv1alpha1.TAPDevice{Name: "tap0"}
		_, err := statusFromNic(nic)
		g.Expect(err).To(HaveOccurred())
	})
}

func TestFormatBDF(t *testing.T) {
	g := NewWithT(t)
	g.Expect(formatBDF(&apinetv1alpha1.PCIAddress{Domain: "0000", Bus: "3b", Slot: "00", Function: "1"})).To(Equal("0000:3b:00.1"))
	g.Expect(formatBDF(&apinetv1alpha1.PCIAddress{Bus: "3b", Slot: "00", Function: "1"})).To(Equal("3b:00.1"))
}

func TestNicName(t *testing.T) {
	g := NewWithT(t)
	g.Expect(NicName("machine", "nic")).To(Equal(NicName("machine", "nic")))
	g.Expect(NicName("machine", "nic")).NotTo(Equal(NicName("machine", "other")))
	g.Expect(NicName("machine", "nic")).NotTo(Equal(NicName("other", "nic")))
}

func TestDelete(t *testing.T) {
	newNic := func(ns, name string) *apinetv1alpha1.NetworkInterface {
		return &apinetv1alpha1.NetworkInterface{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		}
	}

	newPlugin := func(objs ...client.Object) *Plugin {
		return &Plugin{
			client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		}
	}

	exists := func(g *WithT, p *Plugin, ns, name string) bool {
		err := p.client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: name}, &apinetv1alpha1.NetworkInterface{})
		if err == nil {
			return true
		}
		g.Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
		return false
	}

	spec := &plugin.NetworkInterfaceSpec{
		Name:      "nic",
		NetworkId: provider.GetNetworkID("ns", "network", "id", "uid"),
	}

	t.Run("deletes the nic of the machine", func(t *testing.T) {
		g := NewWithT(t)
		p := newPlugin(
			newNic("ns", NicName("machine", "nic")),
			newNic("ns", NicName("machine", "other")),
			newNic("ns", NicName("other", "nic")),
			newNic("other-ns", NicName("machine", "nic")),
		)

		g.Expect(p.Delete(context.Background(), spec, "machine")).To(Succeed())

		g.Expect(exists(g, p, "ns", NicName("machine", "nic"))).To(BeFalse())
		g.Expect(exists(g, p, "ns", NicName("machine", "other"))).To(BeTrue())
		g.Expect(exists(g, p, "ns", NicName("other", "nic"))).To(BeTrue())
		g.Expect(exists(g, p, "other-ns", NicName("machine", "nic"))).To(BeTrue())
	})

	t.Run("missing nic is success", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(newPlugin().Delete(context.Background(), spec, "machine")).To(Succeed())
	})

	t.Run("invalid network id is an error", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(newPlugin().Delete(context.Background(), &plugin.NetworkInterfaceSpec{Name: "nic", NetworkId: "bogus"}, "machine")).To(HaveOccurred())
		g.Expect(newPlugin().Delete(context.Background(), &plugin.NetworkInterfaceSpec{Name: "nic"}, "machine")).To(HaveOccurred())
	})
}
