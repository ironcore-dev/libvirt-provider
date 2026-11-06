// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NetworkInterfaceDetach", func() {
	It("should detach a network interface from the machine", func(ctx SpecContext) {
		By("creating a machine with network interfaces")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"machinepoolletv1alpha1.MachineUIDLabel": "foobar",
					},
				},
				Spec: &iri.MachineSpec{
					Power: iri.Power_POWER_ON,
					Class: machineClassx3xlarge,
					NetworkInterfaces: []*iri.NetworkInterface{
						{
							Name: "nic-1",
						},
						{
							Name: "nic-2",
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())
		DeferCleanup(cleanupMachine(createResp.Machine.Metadata.Id))

		By("ensuring the correct creation response")
		Expect(createResp).Should(SatisfyAll(
			HaveField("Machine.Metadata.Id", Not(BeEmpty())),
			HaveField("Machine.Spec.NetworkInterfaces",
				ConsistOf(
					&iri.NetworkInterface{
						Name: "nic-1"},
					&iri.NetworkInterface{
						Name: "nic-2",
					},
				)),
		))

		By("ensuring the machine gets created in the store")
		Eventually(func(g Gomega) {
			msList, err := machineStore.List(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(msList).NotTo(BeEmpty())
			g.Expect(msList).To(HaveLen(1))
		}).Should(Succeed())

		By("detaching nic-1 network interface from the machine")
		detachNetworkResp, err := machineClient.DetachNetworkInterface(ctx, &iri.DetachNetworkInterfaceRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Name:      "nic-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(detachNetworkResp).NotTo(BeNil())

		By("ensuring nic information is updated in the store")
		Eventually(func(g Gomega) *api.Machine {
			msList, err := machineStore.List(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(msList).NotTo(BeEmpty())
			g.Expect(msList).To(HaveLen(1))
			return msList[0]
		}).Should(SatisfyAll(
			HaveField("Spec.NetworkInterfaces", ConsistOf(
				SatisfyAll(
					HaveField("Name", "nic-1"),
					HaveField("DeletionTimestamp", Not(BeNil())),
				),
				SatisfyAll(
					HaveField("Name", "nic-2"),
					HaveField("DeletionTimestamp", BeNil()),
				),
			)),
		))
	})

	It("should not report a detached network interface in the machine spec", func(ctx SpecContext) {
		By("creating a machine with network interfaces")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"machinepoolletv1alpha1.MachineUIDLabel": "foobar",
					},
				},
				Spec: &iri.MachineSpec{
					Power: iri.Power_POWER_ON,
					Class: machineClassx3xlarge,
					NetworkInterfaces: []*iri.NetworkInterface{
						{
							Name: "nic-1",
						},
						{
							Name: "nic-2",
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())
		DeferCleanup(cleanupMachine(createResp.Machine.Metadata.Id))

		By("detaching nic-1 network interface from the machine")
		detachNetworkResp, err := machineClient.DetachNetworkInterface(ctx, &iri.DetachNetworkInterfaceRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Name:      "nic-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(detachNetworkResp).NotTo(BeNil())

		By("ensuring the detached network interface is not listed in the machine spec")
		listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
			Filter: &iri.MachineFilter{
				Id: createResp.Machine.Metadata.Id,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(listResp.Machines).To(HaveLen(1))
		Expect(listResp.Machines[0].Spec.NetworkInterfaces).To(ConsistOf(
			HaveField("Name", "nic-2"),
		))

		By("detaching nic-1 network interface again")
		detachNetworkResp, err = machineClient.DetachNetworkInterface(ctx, &iri.DetachNetworkInterfaceRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Name:      "nic-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(detachNetworkResp).NotTo(BeNil())
	})
})
