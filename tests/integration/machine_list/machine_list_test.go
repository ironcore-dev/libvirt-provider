// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ListMachine", func() {
	It("should list machines", func(ctx SpecContext) {
		By("creating a machine")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"machine": "list",
					},
				},
				Spec: &iri.MachineSpec{
					Power: iri.Power_POWER_ON,
					Class: machineClassx3xlarge,
					Volumes: []*iri.Volume{
						{
							Name: "disk-1",
							EmptyDisk: &iri.EmptyDisk{
								SizeBytes: 5368709120,
							},
							Device: "oda",
						},
					},
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

		By("listing machines using machine Id")
		Eventually(func(g Gomega) iri.MachineState {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listResp.Machines).NotTo(BeEmpty())
			g.Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status.State
		}).Should(Equal(iri.MachineState_MACHINE_RUNNING))

		By("listing machines using correct Label selector")
		listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
			Filter: &iri.MachineFilter{
				LabelSelector: map[string]string{
					"machine": "list",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(listResp.Machines).NotTo(BeEmpty())
		Expect(listResp.Machines).Should(HaveLen(1))
		Expect(listResp.Machines[0].Status.State).Should(Equal(iri.MachineState_MACHINE_RUNNING))

		By("listing machines using incorrect Label selector")
		listResp, err = machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
			Filter: &iri.MachineFilter{
				LabelSelector: map[string]string{
					"foo": "wrong",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(listResp).NotTo(BeNil())
		Expect(listResp.Machines).To(BeEmpty())
	})
})
