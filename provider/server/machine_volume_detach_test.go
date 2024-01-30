// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"time"

	"github.com/digitalocean/go-libvirt"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"
)

var _ = Describe("DetachVolume", func() {
	It("should correctly detach volume from machine", func(ctx SpecContext) {
		By("creating a machine with two empty disks")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: &iri.MachineSpec{
					Power: iri.Power_POWER_ON,
					Image: &iri.ImageSpec{
						Image: osImage,
					},
					Class: machineClassx3xlarge,
					Volumes: []*iri.Volume{
						{
							Name: "disk-1",
							EmptyDisk: &iri.EmptyDisk{
								SizeBytes: emptyDiskSize,
							},
							Device: "oda",
						},
						{
							Name: "disk-2",
							EmptyDisk: &iri.EmptyDisk{
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

		DeferCleanup(func(ctx SpecContext) {
			Eventually(func() bool {
				_, err := machineClient.DeleteMachine(ctx, &iri.DeleteMachineRequest{MachineId: createResp.Machine.Metadata.Id})
				Expect(err).ShouldNot(HaveOccurred())
				_, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(createResp.Machine.Metadata.Id))
				return libvirt.IsNotFound(err)
			}).Should(BeTrue())
		})

		By("ensuring domain and domain XML is created for machine")
		var domain libvirt.Domain
		Eventually(func() error {
			domain, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(createResp.Machine.Metadata.Id))
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(domainXMLData).NotTo(BeEmpty())

		By("ensuring domain for machine is in running state")
		Eventually(func() libvirt.DomainState {
			domainState, _, err := libvirtConn.DomainGetState(domain, 0)
			Expect(err).NotTo(HaveOccurred())
			return libvirt.DomainState(domainState)
		}).Should(Equal(libvirt.DomainRunning))

		By("ensuring machine is in running state and other status fields have been updated")
		Eventually(func() *iri.MachineStatus {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).NotTo(BeEmpty())
			Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status
		}).Should(SatisfyAll(
			HaveField("Volumes", ContainElements(
				&iri.VolumeStatus{
					Name:   "disk-1",
					Handle: "libvirt-provider.ironcore.dev/empty-disk/disk-1",
					State:  iri.VolumeState_VOLUME_ATTACHED,
				},
				&iri.VolumeStatus{
					Name:   "disk-2",
					Handle: "libvirt-provider.ironcore.dev/empty-disk/disk-2",
					State:  iri.VolumeState_VOLUME_ATTACHED,
				})),
			HaveField("State", Equal(iri.MachineState_MACHINE_RUNNING)),
		))

		By("ensuring both the empty disks are attached to a machine domain")
		var disks []libvirtxml.DomainDisk
		Eventually(func() int {
			domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
			Expect(err).NotTo(HaveOccurred())
			domainXML := &libvirtxml.Domain{}
			err = domainXML.Unmarshal(domainXMLData)
			Expect(err).NotTo(HaveOccurred())
			disks = domainXML.Devices.Disks
			return len(disks)
		}).Should(Equal(3))
		Expect(disks[0].Serial).To(HavePrefix("oda"))
		Expect(disks[1].Serial).To(HavePrefix("odb"))

		// wait for boot image to be available before detaching volume
		time.Sleep(20 * time.Second)

		By("detaching disk-1 from machine")
		detachResp, err := machineClient.DetachVolume(ctx, &iri.DetachVolumeRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Name:      "disk-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(detachResp).NotTo(BeNil())

		By("ensuring disk-1 is unplugged from a machine domain")
		Eventually(func() int {
			domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
			Expect(err).NotTo(HaveOccurred())
			domainXML := &libvirtxml.Domain{}
			err = domainXML.Unmarshal(domainXMLData)
			Expect(err).NotTo(HaveOccurred())
			disks = domainXML.Devices.Disks
			return len(disks)
		}).Should(Equal(2))

		By("ensuring detached volume have been updated in machine status field")
		Eventually(func() *iri.MachineStatus {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).NotTo(BeEmpty())
			Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status
		}).Should(SatisfyAll(
			HaveField("Volumes", ContainElements(
				&iri.VolumeStatus{
					Name:   "disk-2",
					Handle: "libvirt-provider.ironcore.dev/empty-disk/disk-2",
					State:  iri.VolumeState_VOLUME_ATTACHED,
				})),
			HaveField("State", Equal(iri.MachineState_MACHINE_RUNNING)),
		))
	})
})
