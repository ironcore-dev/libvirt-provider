// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/provider-utils/claimutils/pci"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"
)

var _ = Describe("MachineController with GPUs", func() {
	It("should convert two claimed GPUs to libvirt host devices with correct PCI addresses", func(ctx SpecContext) {
		By("creating a machine with claimed GPUs")
		machine := &api.Machine{
			Spec: api.MachineSpec{
				Gpu: []pci.Address{
					{Domain: 0, Bus: 1, Slot: 0, Function: 0},
					{Domain: 0, Bus: 2, Slot: 0, Function: 1},
				},
			},
		}

		By("converting claimed GPUs to host devices")
		hostDevs := claimedGPUsToHostDevs(machine)

		By("ensuring the correct host devices are returned")
		Expect(hostDevs).To(HaveLen(2))
		Expect(hostDevs).To(ContainElements(
			libvirtxml.DomainHostdev{
				Alias: &libvirtxml.DomainAlias{
					Name: "gpu0",
				},
				Managed: "yes",
				SubsysPCI: &libvirtxml.DomainHostdevSubsysPCI{
					Source: &libvirtxml.DomainHostdevSubsysPCISource{
						Address: &libvirtxml.DomainAddressPCI{
							Domain:   &machine.Spec.Gpu[0].Domain,
							Bus:      &machine.Spec.Gpu[0].Bus,
							Slot:     &machine.Spec.Gpu[0].Slot,
							Function: &machine.Spec.Gpu[0].Function,
						},
					},
				},
				Address: &libvirtxml.DomainAddress{
					PCI: &libvirtxml.DomainAddressPCI{},
				},
			},
			libvirtxml.DomainHostdev{
				Alias: &libvirtxml.DomainAlias{
					Name: "gpu1",
				},
				Managed: "yes",
				SubsysPCI: &libvirtxml.DomainHostdevSubsysPCI{
					Source: &libvirtxml.DomainHostdevSubsysPCISource{
						Address: &libvirtxml.DomainAddressPCI{
							Domain:   &machine.Spec.Gpu[1].Domain,
							Bus:      &machine.Spec.Gpu[1].Bus,
							Slot:     &machine.Spec.Gpu[1].Slot,
							Function: &machine.Spec.Gpu[1].Function,
						},
					},
				},
				Address: &libvirtxml.DomainAddress{
					PCI: &libvirtxml.DomainAddressPCI{},
				},
			},
		))
	})

	It("should return empty host devices for empty GPU slice", func(ctx SpecContext) {
		By("creating a machine with 0 claimed GPUs")
		machine := &api.Machine{
			Spec: api.MachineSpec{
				Gpu: []pci.Address{},
			},
		}

		By("converting claimed GPUs to host devices")
		hostDevs := claimedGPUsToHostDevs(machine)

		By("ensuring no host devices are returned")
		Expect(hostDevs).To(HaveLen(0))
	})

	It("should return empy host devices for nil GPU field", func(ctx SpecContext) {
		By("creating a machine with nil Gu field")
		machine := &api.Machine{
			Spec: api.MachineSpec{},
		}

		By("converting claimed GPUs to host devices")
		hostDevs := claimedGPUsToHostDevs(machine)

		By("ensuring no host devices are returned")
		Expect(hostDevs).To(HaveLen(0))
	})
})
