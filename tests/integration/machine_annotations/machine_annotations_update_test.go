// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpdateMachineAnnotations", func() {
	It("should update machine annotations", func(ctx SpecContext) {
		ignitionData := []byte("urjhikmnbdjfkknhhdddeee")
		By("creating a machine")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"machine": "annotations"},
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

		DeferCleanup(machineClient.DeleteMachine, &iri.DeleteMachineRequest{
			MachineId: createResp.Machine.Metadata.Id,
		})

		By("ensuring domain and domain XML is created and machine is running")
		assertMachineIsRunning(createResp.Machine.Metadata.Id)

		By("updating machine annotations")
		_, err = machineClient.UpdateMachineAnnotations(ctx, &iri.UpdateMachineAnnotationsRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Annotations: map[string]string{
				"machinepoolletv1alpha1.MachineUIDLabel": "fooUpdatedAnnotation",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) bool {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listResp.Machines).NotTo(BeEmpty())
			g.Expect(listResp.Machines).Should(HaveLen(1))

			machineStatus := listResp.Machines[0].Status
			return machineStatus.State == iri.MachineState_MACHINE_RUNNING
		}).Should(BeTrue())

		By("ensuring correct annotations")
		Eventually(func(g Gomega) *irimeta.ObjectMetadata {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listResp.Machines).NotTo(BeEmpty())
			g.Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Metadata
		}).Should(SatisfyAll(
			HaveField("Annotations", Equal(map[string]string{
				"machinepoolletv1alpha1.MachineUIDLabel": "fooUpdatedAnnotation",
			})),
		))
	})
})
