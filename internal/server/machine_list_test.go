// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
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
		By("creating machines")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"machinepoolletv1alpha1.MachineUIDLabel": "foo",
					},
				},
				Spec: &iri.MachineSpec{
					Power: iri.Power_POWER_ON,
					Class: machineClassx3xlarge,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())
		DeferCleanup(func() {
			err := machineStore.Delete(ctx, createResp.Machine.Metadata.Id)
			Expect(err).NotTo(HaveOccurred())
		})

		createMachineResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"machinepoolletv1alpha1.MachineUIDLabel": "bar",
					},
				},
				Spec: &iri.MachineSpec{
					Power: iri.Power_POWER_ON,
					Class: machineClassx3xlarge,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createMachineResp).NotTo(BeNil())
		DeferCleanup(func() {
			err := machineStore.Delete(ctx, createMachineResp.Machine.Metadata.Id)
			Expect(err).NotTo(HaveOccurred())
		})

		By("ensuring the machines gets created in the store")
		Eventually(func(g Gomega) {
			msList, err := machineStore.List(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(msList).NotTo(BeEmpty())
			g.Expect(msList).To(HaveLen(2))
		}).Should(Succeed())

		By("listing machines using machine Id")
		listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
			Filter: &iri.MachineFilter{
				Id: createResp.Machine.Metadata.Id,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(listResp.Machines).NotTo(BeEmpty())
		Expect(listResp.Machines).Should(HaveLen(1))

		By("listing machines using correct Label selector")
		listResp, err = machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
			Filter: &iri.MachineFilter{
				LabelSelector: map[string]string{
					"machinepoolletv1alpha1.MachineUIDLabel": "foo",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(listResp.Machines).NotTo(BeEmpty())
		Expect(listResp.Machines).Should(HaveLen(1))

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
