// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateMachine", func() {
	It("should create a machine without boot image, volume and network interface", func(ctx SpecContext) {
		By("creating a machine without boot image, volume and network interface")
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
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

		By("ensuring the correct creation response")
		Expect(createResp).Should(SatisfyAll(
			HaveField("Machine.Metadata.Id", Not(BeEmpty())),
			HaveField("Machine.Spec.Power", iri.Power_POWER_ON),
			HaveField("Machine.Spec.Class", machineClassx3xlarge),
			HaveField("Machine.Spec.IgnitionData", BeNil()),
			HaveField("Machine.Spec.Volumes", BeNil()),
			HaveField("Machine.Spec.NetworkInterfaces", BeNil()),
			HaveField("Machine.Status.ObservedGeneration", BeZero()),
			HaveField("Machine.Status.State", Equal(iri.MachineState_MACHINE_PENDING)),
			HaveField("Machine.Status.ImageRef", BeEmpty()),
			HaveField("Machine.Status.Volumes", BeNil()),
			HaveField("Machine.Status.NetworkInterfaces", BeNil()),
		))

		By("ensuring the machine gets created in the store")
		Eventually(func(g Gomega) *api.Machine {
			msList, err := machineStore.List(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(msList).NotTo(BeEmpty())
			g.Expect(msList).To(HaveLen(1))
			return msList[0]
		}).Should(SatisfyAll(
			HaveField("Spec.MemoryBytes", int64(8589934592)),
			HaveField("Spec.Cpu", int64(4)),
		))
	})

	It("should create a machine without boot image, but volume and nic", func(ctx SpecContext) {
		By("creating a machine without boot image")
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
					Volumes: []*iri.Volume{
						{
							Name: "disk-1",
							LocalDisk: &iri.LocalDisk{
								SizeBytes: emptyDiskSize,
							},
							Device: "oda",
						},
					},
					NetworkInterfaces: []*iri.NetworkInterface{
						{
							Name: "nic-1",
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

		By("ensuring the correct creation response")
		Expect(createResp).Should(SatisfyAll(
			HaveField("Machine.Metadata.Id", Not(BeEmpty())),
			HaveField("Machine.Spec.Power", iri.Power_POWER_ON),
			HaveField("Machine.Spec.Class", machineClassx3xlarge),
			HaveField("Machine.Spec.IgnitionData", BeNil()),
			HaveField("Machine.Spec.Volumes", ContainElement(&iri.Volume{
				Name: "disk-1",
				LocalDisk: &iri.LocalDisk{
					SizeBytes: emptyDiskSize,
				},
				Device: "oda",
			})),
			HaveField("Machine.Spec.NetworkInterfaces", ContainElement(&iri.NetworkInterface{
				Name: "nic-1",
			})),
			HaveField("Machine.Status.ObservedGeneration", BeZero()),
			HaveField("Machine.Status.State", Equal(iri.MachineState_MACHINE_PENDING)),
			HaveField("Machine.Status.ImageRef", BeEmpty()),
			HaveField("Machine.Status.Volumes", BeNil()),
			HaveField("Machine.Status.NetworkInterfaces", BeNil()),
		))

		By("ensuring the machine gets created in the store")
		Eventually(func(g Gomega) *api.Machine {
			msList, err := machineStore.List(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(msList).NotTo(BeEmpty())
			g.Expect(msList).To(HaveLen(1))
			return msList[0]
		}).Should(SatisfyAll(
			HaveField("Spec.MemoryBytes", int64(8589934592)),
			HaveField("Spec.Cpu", int64(4)),
			HaveField("Spec.Volumes", ContainElement(SatisfyAll(
				HaveField("Name", "disk-1"),
				HaveField("Device", "oda"),
			))),
			HaveField("Spec.NetworkInterfaces", ContainElement(SatisfyAll(
				HaveField("Name", "nic-1"),
			))),
		))
	})
})
