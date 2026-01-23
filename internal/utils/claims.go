// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/provider-utils/claimutils/pci"
	hostutils "github.com/ironcore-dev/provider-utils/storeutils/host"
)

func GetClaimedPCIAddressesFromMachineStore(ctx context.Context, machineStore *hostutils.Store[*api.Machine]) ([]pci.Address, error) {
	machines, err := machineStore.List(ctx)
	if err != nil {
		return nil, err
	}

	var pciAddrs []pci.Address
	for _, machine := range machines {
		if machine.Spec.Gpu != nil {
			pciAddrs = append(pciAddrs, machine.Spec.Gpu...)
		}
	}
	return pciAddrs, nil
}
