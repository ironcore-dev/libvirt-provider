// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"path/filepath"
	"time"

	"github.com/digitalocean/go-libvirt"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DeleteMachine", func() {

	It("should delete a machine with graceful shutdown", func(ctx SpecContext) {
		By("creating a machine using squashfs os image")
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
						Image: squashfsOSImage,
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
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

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

		By("ensuring machine is in running state")
		Eventually(func() iri.MachineState {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).NotTo(BeEmpty())
			return listResp.Machines[0].Status.State
		}).Should(Equal(iri.MachineState_MACHINE_RUNNING))

		//allow some time for the vm to boot properly
		time.Sleep(30 * time.Second)

		By("deleting the machine")
		_, err = machineClient.DeleteMachine(ctx, &iri.DeleteMachineRequest{
			MachineId: createResp.Machine.Metadata.Id,
		})
		Expect(err).NotTo(HaveOccurred())

		By("ensuring machine is in Terminating state after delete")
		Eventually(func() iri.MachineState {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			return listResp.Machines[0].Status.State
		}).Should(Equal(iri.MachineState_MACHINE_TERMINATING))

		By("ensuring machine is gracefully shutdown")
		Eventually(func() int {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			return len(listResp.Machines)
		}).Within(gracefulShutdownTimeout).ProbeEvery(2 * time.Second).Should(BeZero()) // ProbeEvery has to be ideally less than or equal to half of ResyncIntervalGarbageCollector

		By("ensuring domain and domain XML is deleted for machine")
		domain, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(createResp.Machine.Metadata.Id))
		Expect(libvirt.IsNotFound(err)).Should(BeTrue())
		domainXMLData, err = libvirtConn.DomainGetXMLDesc(domain, 0)
		Expect(domainXMLData).To(BeEmpty())

		By("ensuring the respective machine's file is cleaned from machines directory")
		machineFile := filepath.Join(tempDir, "libvirt-provider", "machines", createResp.Machine.Metadata.Id)
		Expect(machineFile).NotTo(BeAnExistingFile())
	})

	It("should delete a machine without graceful shutdown", func(ctx SpecContext) {
		By("creating a machine which may not boot properly")
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
								SizeBytes: emptyDiskSize,
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
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).NotTo(BeEmpty())
			return listResp.Machines[0].Status.State
		}).Should(Equal(iri.MachineState_MACHINE_RUNNING))

		By("deleting the machine")
		_, err = machineClient.DeleteMachine(ctx, &iri.DeleteMachineRequest{
			MachineId: createResp.Machine.Metadata.Id,
		})
		Expect(err).NotTo(HaveOccurred())

		By("ensuring machine is in Terminating state after delete")
		Eventually(func() iri.MachineState {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status.State
		}).Should(Equal(iri.MachineState_MACHINE_TERMINATING))

		By("ensuring machine is not deleted until gracefulShutdownTimeout")
		Consistently(func() bool {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).Should(HaveLen(1))
			return err == nil && len(listResp.Machines) == 1
		}, gracefulShutdownTimeout, consistentlyDuration).Should(BeTrue())

		By("ensuring machine is deleted after gracefulShutdownTimeout")
		Eventually(func() int {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			return len(listResp.Machines)
		}).Should(BeZero())

		By("ensuring domain and domain XML is deleted for machine")
		domain, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(createResp.Machine.Metadata.Id))
		Expect(libvirt.IsNotFound(err)).Should(BeTrue())
		domainXMLData, err = libvirtConn.DomainGetXMLDesc(domain, 0)
		Expect(domainXMLData).To(BeEmpty())

		By("ensuring the respective machine's file is cleaned from machines directory")
		machineFile := filepath.Join(tempDir, "libvirt-provider", "machines", createResp.Machine.Metadata.Id)
		Expect(machineFile).NotTo(BeAnExistingFile())
	})
})
