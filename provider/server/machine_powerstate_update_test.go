// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"github.com/digitalocean/go-libvirt"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpdateMachinePower", func() {
	It("should update machine power state", func(ctx SpecContext) {
		ignitionData := []byte("urjhikmnbdjfkknhhdddeee")
		By("creating a machine")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"machinepoolletv1alpha1.MachineUIDLabel": "foobar",
					},
				},
				Spec: &iri.MachineSpec{
					Power:             iri.Power_POWER_ON,
					Class:             machineClassx3xlarge,
					IgnitionData:      ignitionData,
					Volumes:           nil,
					NetworkInterfaces: nil,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

		DeferCleanup(func(ctx SpecContext) {
			Eventually(func() bool {
				_, err := machineClient.DeleteMachine(ctx, &iri.DeleteMachineRequest{MachineId: createResp.Machine.Metadata.Id})
				Expect(err).ShouldNot(HaveOccurred())
				_, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(createResp.Machine.Metadata.Id))
				return libvirt.IsNotFound(err)
			}).Should(BeTrue())
		})

		By("updating power state of machine")
		_, err = machineClient.UpdateMachinePower(ctx, &iri.UpdateMachinePowerRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Power:     iri.Power_POWER_OFF,
		})
		Expect(err).NotTo(HaveOccurred())

		By("ensuring the correct power state")
		Eventually(func() *iri.MachineSpec {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).NotTo(BeEmpty())
			return listResp.Machines[0].Spec
		}).Should(SatisfyAll(
			HaveField("Power", Equal(iri.Power_POWER_OFF)),
		))
	})
})
