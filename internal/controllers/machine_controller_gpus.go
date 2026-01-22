// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"fmt"

	"github.com/ironcore-dev/libvirt-provider/api"
	"libvirt.org/go/libvirtxml"
)

func clamedGPUsToHostDevs(machine *api.Machine) []libvirtxml.DomainHostdev {
	hostDevs := make([]libvirtxml.DomainHostdev, len(machine.Spec.Gpu))

	for i, gpuAddr := range machine.Spec.Gpu {
		domain := gpuAddr.Domain
		bus := gpuAddr.Bus
		slot := gpuAddr.Slot
		function := gpuAddr.Function

		hostDevs[i] = libvirtxml.DomainHostdev{
			Alias: &libvirtxml.DomainAlias{
				Name: fmt.Sprintf("gpu%d", i),
			},
			Managed: "yes",
			SubsysPCI: &libvirtxml.DomainHostdevSubsysPCI{
				Source: &libvirtxml.DomainHostdevSubsysPCISource{
					Address: &libvirtxml.DomainAddressPCI{
						Domain:   &domain,
						Bus:      &bus,
						Slot:     &slot,
						Function: &function,
					},
				},
			},
			Address: &libvirtxml.DomainAddress{
				PCI: &libvirtxml.DomainAddressPCI{},
			},
		}
	}

	return hostDevs
}
