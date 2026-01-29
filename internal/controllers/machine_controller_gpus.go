// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/provider-utils/claimutils/claim"
	"github.com/ironcore-dev/provider-utils/claimutils/gpu"
	"github.com/ironcore-dev/provider-utils/claimutils/pci"
	"libvirt.org/go/libvirtxml"
)

func ClaimedGPUsToHostDevs(machine *api.Machine) []libvirtxml.DomainHostdev {
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

func (r *MachineReconciler) releaseResourceClaims(ctx context.Context, log logr.Logger, pciAddrs []pci.Address) error {
	log.V(2).Info("Releasing GPU claims", "pciAddresses", pciAddrs)
	claims := claim.Claims{
		v1alpha1.ResourceName(api.NvidiaGPUPlugin): gpu.NewGPUClaim(pciAddrs),
	}
	err := r.resourceClaimer.Release(ctx, claims)
	if err != nil {
		log.Error(err, "Failed to release GPU claims", "pciAddresses", pciAddrs)
		return err
	}
	log.V(2).Info("Successfully released GPU claims", "pciAddresses", pciAddrs)
	return nil
}
