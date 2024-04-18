// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"github.com/digitalocean/go-libvirt"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	osImage = "ghcr.io/ironcore-dev/ironcore-image/gardenlinux:rootfs-dev-20231206-v1"
)

func assertMachineIsRunning(machineID string) {
	GinkgoHelper()
	By("ensuring domain and domain XML is created for machine")
	var domain libvirt.Domain

	Eventually(func() (err error) {
		domain, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machineID))
		return err
	}).Should(Succeed())
	domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
	Expect(err).NotTo(HaveOccurred())
	Expect(domainXMLData).NotTo(BeEmpty())

	By("ensuring domain for machine is in running state")
	Eventually(func(g Gomega) libvirt.DomainState {
		domainState, _, err := libvirtConn.DomainGetState(domain, 0)
		g.Expect(err).NotTo(HaveOccurred())
		return libvirt.DomainState(domainState)
	}).Should(Equal(libvirt.DomainRunning))
}

var _ = Describe("CreateMachine", func() {
	It("should create a machine simple machine", func(ctx SpecContext) {
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

		DeferCleanup(machineClient.DeleteMachine, &iri.DeleteMachineRequest{
			MachineId: createResp.Machine.Metadata.Id,
		})

		By("ensuring the correct creation response")
		Expect(createResp).Should(SatisfyAll(
			HaveField("Machine.Metadata.Id", Not(BeEmpty())),
			HaveField("Machine.Spec.Power", iri.Power_POWER_ON),
			HaveField("Machine.Spec.Image", BeNil()),
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

		By("ensuring domain and domain XML is created and machine is running")
		assertMachineIsRunning(createResp.Machine.Metadata.Id)

	})

	It("should create a machine with volumes and nics", func(ctx SpecContext) {
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
							EmptyDisk: &iri.EmptyDisk{
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

		DeferCleanup(machineClient.DeleteMachine, &iri.DeleteMachineRequest{
			MachineId: createResp.Machine.Metadata.Id,
		})

		By("ensuring the correct creation response")
		Expect(createResp).Should(SatisfyAll(
			HaveField("Machine.Metadata.Id", Not(BeEmpty())),
			HaveField("Machine.Spec.Power", iri.Power_POWER_ON),
			HaveField("Machine.Spec.Image", BeNil()),
			HaveField("Machine.Spec.Class", machineClassx3xlarge),
			HaveField("Machine.Spec.IgnitionData", BeNil()),
			HaveField("Machine.Spec.Volumes", ContainElement(&iri.Volume{
				Name: "disk-1",
				EmptyDisk: &iri.EmptyDisk{
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

		By("ensuring domain and domain XML is created and machine is running")
		assertMachineIsRunning(createResp.Machine.Metadata.Id)

		By("ensuring machine is in running state and other status fields have been updated")
		Eventually(func(g Gomega) *iri.MachineStatus {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listResp.Machines).NotTo(BeEmpty())
			g.Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status
		}).Should(SatisfyAll(
			HaveField("ObservedGeneration", BeZero()),
			HaveField("ImageRef", BeEmpty()),
			HaveField("Volumes", ContainElement(&iri.VolumeStatus{
				Name:   "disk-1",
				Handle: "libvirt-provider.ironcore.dev/empty-disk/disk-1",
				State:  iri.VolumeState_VOLUME_ATTACHED,
			})),
			HaveField("NetworkInterfaces", ContainElement(&iri.NetworkInterfaceStatus{
				Name:  "nic-1",
				State: iri.NetworkInterfaceState_NETWORK_INTERFACE_ATTACHED,
			})),
			HaveField("State", Equal(iri.MachineState_MACHINE_RUNNING)),
		))
	})

	ignitionData := []byte("urjhikmnbdjfkknhhdddeee")
	It("should create a machine with boot image and single empty disk", func(ctx SpecContext) {
		By("creating a machine with boot image and single empty disk")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"machinepoolletv1alpha1.MachineUIDLabel": "foobar",
					},
				},
				Spec: &iri.MachineSpec{
					Power: iri.Power_POWER_ON,
					Image: &iri.ImageSpec{
						Image: osImage,
					},
					Class:        machineClassx3xlarge,
					IgnitionData: ignitionData,
					Volumes: []*iri.Volume{
						{
							Name: "disk-1",
							EmptyDisk: &iri.EmptyDisk{
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

		DeferCleanup(machineClient.DeleteMachine, &iri.DeleteMachineRequest{
			MachineId: createResp.Machine.Metadata.Id,
		})

		By("ensuring the correct creation response")
		Expect(createResp).Should(SatisfyAll(
			HaveField("Machine.Metadata.Id", Not(BeEmpty())),
			HaveField("Machine.Spec.Power", iri.Power_POWER_ON),
			HaveField("Machine.Spec.Image.Image", Equal(osImage)),
			HaveField("Machine.Spec.Class", machineClassx3xlarge),
			HaveField("Machine.Spec.IgnitionData", Equal(ignitionData)),
			HaveField("Machine.Spec.Volumes", ContainElement(&iri.Volume{
				Name:   "disk-1",
				Device: "oda",
				EmptyDisk: &iri.EmptyDisk{
					SizeBytes: emptyDiskSize,
				},
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

		By("ensuring domain and domain XML is created and machine is running")
		assertMachineIsRunning(createResp.Machine.Metadata.Id)

		By("ensuring machine is in running state and other status fields have been updated")
		Eventually(func(g Gomega) *iri.MachineStatus {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listResp.Machines).NotTo(BeEmpty())
			g.Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status
		}).Should(SatisfyAll(
			HaveField("ObservedGeneration", BeZero()),
			HaveField("ImageRef", BeEmpty()),
			HaveField("Volumes", ContainElement(&iri.VolumeStatus{
				Name:   "disk-1",
				Handle: "libvirt-provider.ironcore.dev/empty-disk/disk-1",
				State:  iri.VolumeState_VOLUME_ATTACHED,
			})),
			HaveField("NetworkInterfaces", ContainElement(&iri.NetworkInterfaceStatus{
				Name:  "nic-1",
				State: iri.NetworkInterfaceState_NETWORK_INTERFACE_ATTACHED,
			})),
			HaveField("State", Equal(iri.MachineState_MACHINE_RUNNING)),
		))
	})

	It("should create a machine with boot image and multiple empty disks", func(ctx SpecContext) {
		By("creating a machine with boot image and multiple empty disks")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"machinepoolletv1alpha1.MachineUIDLabel": "foobar",
					},
				},
				Spec: &iri.MachineSpec{
					Power: iri.Power_POWER_ON,
					Image: &iri.ImageSpec{
						Image: osImage,
					},
					Class:        machineClassx3xlarge,
					IgnitionData: ignitionData,
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

		DeferCleanup(machineClient.DeleteMachine, &iri.DeleteMachineRequest{
			MachineId: createResp.Machine.Metadata.Id,
		})

		By("ensuring the correct creation response")
		Expect(createResp).Should(SatisfyAll(
			HaveField("Machine.Metadata.Id", Not(BeEmpty())),
			HaveField("Machine.Spec.Power", iri.Power_POWER_ON),
			HaveField("Machine.Spec.Image.Image", Equal(osImage)),
			HaveField("Machine.Spec.Class", machineClassx3xlarge),
			HaveField("Machine.Spec.IgnitionData", Equal(ignitionData)),
			HaveField("Machine.Spec.Volumes", ContainElements(
				&iri.Volume{
					Name:   "disk-1",
					Device: "oda",
					EmptyDisk: &iri.EmptyDisk{
						SizeBytes: emptyDiskSize,
					},
				},
				&iri.Volume{
					Name:   "disk-2",
					Device: "odb",
					EmptyDisk: &iri.EmptyDisk{
						SizeBytes: emptyDiskSize,
					},
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

		By("ensuring domain and domain XML is created and machine is running")
		assertMachineIsRunning(createResp.Machine.Metadata.Id)

		By("ensuring machine is in running state and other status fields have been updated")
		Eventually(func(g Gomega) *iri.MachineStatus {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listResp.Machines).NotTo(BeEmpty())
			g.Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status
		}).Should(SatisfyAll(
			HaveField("ObservedGeneration", BeZero()),
			HaveField("ImageRef", BeEmpty()),
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
			HaveField("NetworkInterfaces", ContainElement(&iri.NetworkInterfaceStatus{
				Name:  "nic-1",
				State: iri.NetworkInterfaceState_NETWORK_INTERFACE_ATTACHED,
			})),
			HaveField("State", Equal(iri.MachineState_MACHINE_RUNNING)),
		))
	})
})
