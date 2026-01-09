// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DetachVolume", func() {
	It("should correctly detach volume from machine", func(ctx SpecContext) {
		By("creating a machine")
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
						{
							Name: "disk-2",
							LocalDisk: &iri.LocalDisk{
								SizeBytes: emptyDiskSize,
							},
							Device: "odb",
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())
		DeferCleanup(func() {
			err := machineStore.Delete(ctx, createResp.Machine.Metadata.Id)
			Expect(err).NotTo(HaveOccurred())
		})

		By("ensuring the correct creation response")
		Expect(createResp).Should(SatisfyAll(
			HaveField("Machine.Metadata.Id", Not(BeEmpty())),
			HaveField("Machine.Spec.Volumes", ContainElements(&iri.Volume{
				Name: "disk-1",
				LocalDisk: &iri.LocalDisk{
					SizeBytes: emptyDiskSize,
				},
				Device: "oda",
			},
				&iri.Volume{
					Name: "disk-2",
					LocalDisk: &iri.LocalDisk{
						SizeBytes: emptyDiskSize,
					},
					Device: "odb",
				})),
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
			HaveField("Spec.Volumes", ContainElements(SatisfyAll(
				HaveField("Name", "disk-1"),
				HaveField("Device", "oda"),
			), SatisfyAll(
				HaveField("Name", "disk-2"),
				HaveField("Device", "odb"),
			))),
		))

		By("detaching volume from machine")
		diskDetachResp, err := machineClient.DetachVolume(ctx, &iri.DetachVolumeRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Name:      "disk-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(diskDetachResp).NotTo(BeNil())

		By("ensuring the volumes gets removed in the store")
		Eventually(func(g Gomega) *api.Machine {
			msList, err := machineStore.List(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(msList).NotTo(BeEmpty())
			g.Expect(msList).To(HaveLen(1))
			return msList[0]
		}).Should(
			HaveField("Spec.Volumes", (ContainElements(SatisfyAll(
				HaveField("Name", "disk-2"),
				HaveField("Device", "odb"),
			))),
			))
	})
})
