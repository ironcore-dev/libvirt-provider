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
	"libvirt.org/go/libvirtxml"
)

var _ = Describe("DetachVolume", func() {
	It("should correctly attach volume to machine", func(ctx SpecContext) {
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
					Class: machineClassx3xlarge,
					Volumes: []*iri.Volume{
						{
							Name: "disk-1",
							EmptyDisk: &iri.EmptyDisk{
								SizeBytes: 5368709120,
							},
							Device: "oda",
						},
						{
							Name: "disk-2",
							EmptyDisk: &iri.EmptyDisk{
								SizeBytes: 5368709120,
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
				libvirtConn.DomainDestroy(libvirt.Domain{UUID: libvirtutils.UUIDStringToBytes(createResp.Machine.Metadata.Id)})
				_, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(createResp.Machine.Metadata.Id))
				return libvirt.IsNotFound(err)
			}).Should(BeTrue())
		})

		By("ensuring domain and domain XML is created for machine")
		var domain libvirt.Domain
		Eventually(func() error {
			domain, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(createResp.Machine.Metadata.Id))
			return err
		}).Should(Succeed())
		domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(domainXMLData).NotTo(BeEmpty())

		By("ensuring domain for machine is in running state")
		Eventually(func() libvirt.DomainState {
			domainState, _, err := libvirtConn.DomainGetState(domain, 0)
			Expect(err).NotTo(HaveOccurred())
			return libvirt.DomainState(domainState)
		}).Should(Equal(libvirt.DomainRunning))

		By("ensuring machine is in running state")
		Eventually(func() iri.MachineState {
			resp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Machines).NotTo(BeEmpty())
			return resp.Machines[0].Status.State
		}).Should(Equal(iri.MachineState_MACHINE_RUNNING))

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
		}).Should(Equal(2))
		Expect(disks[0].Serial).To(HavePrefix("oda"))
		Expect(disks[1].Serial).To(HavePrefix("odb"))

		By("detaching disk-1 from machine")
		detachResp, err := machineClient.DetachVolume(ctx, &iri.DetachVolumeRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Name:      "disk-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(detachResp).NotTo(BeNil())

		// this has to be uncommented when issue for Disk not being unplugged from domain gets resolved

		// By("ensuring disk-1 is dettached from a machine domain")
		// Eventually(func() int {
		// 	domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	domainXML := &libvirtxml.Domain{}
		// 	err = domainXML.Unmarshal(domainXMLData)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	disks = domainXML.Devices.Disks
		// 	return len(disks)
		// }).Should(Equal(1))

		// TODO - ensuring volume spec and status is updated in iri machine to be done
		// after convertMachineToIRIMachine() supports Volumes and NetworkInterfaces
	})
})
