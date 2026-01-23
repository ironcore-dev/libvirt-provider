// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"path/filepath"

	"github.com/google/uuid"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/strategy"
	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"
	"github.com/ironcore-dev/provider-utils/claimutils/pci"
	hostutils "github.com/ironcore-dev/provider-utils/storeutils/host"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Claims Utils", func() {
	Context("GetClaimedPCIAddressesFromMachineStore", func() {
		It("should get all claimed PCI addresses from the machine store", func(ctx SpecContext) {
			tempDir := GinkgoT().TempDir()

			machineStore, err := hostutils.NewStore[*api.Machine](hostutils.Options[*api.Machine]{
				NewFunc:        func() *api.Machine { return &api.Machine{} },
				CreateStrategy: strategy.MachineStrategy,
				Dir:            filepath.Join(tempDir, "store", "machines"),
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = machineStore.Create(ctx, &api.Machine{
				Metadata: apiutils.Metadata{
					ID: IdGenerateFunc(uuid.NewString).Generate(),
				},
				Spec: api.MachineSpec{
					Gpu: []pci.Address{{Domain: 0, Bus: 1, Slot: 0, Function: 0}},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			pciAddrs, err := GetClaimedPCIAddressesFromMachineStore(ctx, machineStore)
			Expect(err).NotTo(HaveOccurred())

			Expect(pciAddrs).To(HaveLen(1))
			Expect(pciAddrs[0]).To(Equal(pci.Address{Domain: 0, Bus: 1, Slot: 0, Function: 0}))
		})
	})
})
