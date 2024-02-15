// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"time"

	"github.com/digitalocean/go-libvirt"
	machineiriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimetav1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	volumeiriv1alpha1 "github.com/ironcore-dev/ironcore/iri/apis/volume/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"
)

var _ = Describe("AttachVolume", func() {
	It("should correctly attach an empty disk to the machine", func(ctx SpecContext) {
		By("creating a machine")
		createResp, err := machineClient.CreateMachine(ctx, &machineiriv1alpha1.CreateMachineRequest{
			Machine: &machineiriv1alpha1.Machine{
				Metadata: &irimetav1alpha1.ObjectMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: &machineiriv1alpha1.MachineSpec{
					Power: machineiriv1alpha1.Power_POWER_ON,
					Class: machineClassx3xlarge,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

		DeferCleanup(func(ctx SpecContext) {
			Eventually(func() bool {
				_, err := machineClient.DeleteMachine(ctx, &machineiriv1alpha1.DeleteMachineRequest{MachineId: createResp.Machine.Metadata.Id})
				Expect(err).To(SatisfyAny(
					BeNil(),
					MatchError(ContainSubstring("NotFound")),
				))
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

		By("attaching an empty disk volume to the machine")
		attachEmptyDiskResp, err := machineClient.AttachVolume(ctx, &machineiriv1alpha1.AttachVolumeRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Volume: &machineiriv1alpha1.Volume{
				Name: "disk-1",
				EmptyDisk: &machineiriv1alpha1.EmptyDisk{
					SizeBytes: 1073741824,
				},
				Device: "oda",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(attachEmptyDiskResp).NotTo(BeNil())

		By("ensuring disk is attached to the machine domain")
		var disks []libvirtxml.DomainDisk
		Eventually(func() int {
			domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
			Expect(err).NotTo(HaveOccurred())
			domainXML := &libvirtxml.Domain{}
			Expect(domainXML.Unmarshal(domainXMLData)).Should(Succeed())
			disks = domainXML.Devices.Disks
			return len(disks)
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Equal(1))
		Expect(disks[0].Serial).To(HavePrefix("oda"))

		By("ensuring attached empty disk volume has been updated in machine status field")
		Eventually(func() *machineiriv1alpha1.MachineStatus {
			listResp, err := machineClient.ListMachines(ctx, &machineiriv1alpha1.ListMachinesRequest{
				Filter: &machineiriv1alpha1.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).NotTo(BeEmpty())
			Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status
		}).Should(SatisfyAll(
			HaveField("Volumes", ContainElements(
				&machineiriv1alpha1.VolumeStatus{
					Name:   "disk-1",
					Handle: "libvirt-provider.ironcore.dev/empty-disk/disk-1",
					State:  machineiriv1alpha1.VolumeState_VOLUME_ATTACHED,
				},
			)),
			HaveField("State", Equal(machineiriv1alpha1.MachineState_MACHINE_RUNNING)),
		))
	})

	It("should correctly attach a non-encrypted volume to the machine", func(ctx SpecContext) {
		By("creating a machine")
		createResp, err := machineClient.CreateMachine(ctx, &machineiriv1alpha1.CreateMachineRequest{
			Machine: &machineiriv1alpha1.Machine{
				Metadata: &irimetav1alpha1.ObjectMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: &machineiriv1alpha1.MachineSpec{
					Power: machineiriv1alpha1.Power_POWER_ON,
					Class: machineClassx3xlarge,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

		DeferCleanup(func(ctx SpecContext) {
			Eventually(func() bool {
				_, err := machineClient.DeleteMachine(ctx, &machineiriv1alpha1.DeleteMachineRequest{MachineId: createResp.Machine.Metadata.Id})
				Expect(err).To(SatisfyAny(
					BeNil(),
					MatchError(ContainSubstring("NotFound")),
				))
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

		By("creating a non-encrypted volume")
		createVolResp, err := volumeClient.CreateVolume(ctx, &volumeiriv1alpha1.CreateVolumeRequest{
			Volume: &volumeiriv1alpha1.Volume{
				Metadata: &irimetav1alpha1.ObjectMetadata{
					Id: "non-encr-volume",
				},
				Spec: &volumeiriv1alpha1.VolumeSpec{
					Class: "foo",
					Resources: &volumeiriv1alpha1.VolumeResources{
						StorageBytes: 500 * 1024 * 1024,
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		By("ensuring the correct encrypted volume is created")
		Expect(createVolResp).Should(SatisfyAll(
			HaveField("Volume.Metadata.Id", Not(BeEmpty())),
			HaveField("Volume.Spec.Class", Equal("foo")),
			HaveField("Volume.Spec.Resources.StorageBytes", Equal(int64(500*1024*1024))),
			HaveField("Volume.Spec.Encryption", BeNil()),
			HaveField("Volume.Status.State", Equal(volumeiriv1alpha1.VolumeState_VOLUME_PENDING)),
			HaveField("Volume.Status.Access", BeNil()),
		))

		DeferCleanup(func(ctx SpecContext) {
			Eventually(func() {
				_, err = volumeClient.DeleteVolume(ctx, &volumeiriv1alpha1.DeleteVolumeRequest{
					VolumeId: createVolResp.Volume.Metadata.Id,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		By("ensuring volume is in available state and other status fields have been updated")
		var listVolumeResp *volumeiriv1alpha1.ListVolumesResponse
		Eventually(func() *volumeiriv1alpha1.VolumeStatus {
			listVolumeResp, err = volumeClient.ListVolumes(ctx, &volumeiriv1alpha1.ListVolumesRequest{
				Filter: &volumeiriv1alpha1.VolumeFilter{
					Id: createVolResp.Volume.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listVolumeResp.Volumes).NotTo(BeEmpty())
			return listVolumeResp.Volumes[0].Status
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(SatisfyAll(
			HaveField("State", Equal(volumeiriv1alpha1.VolumeState_VOLUME_AVAILABLE)),
			HaveField("Access", SatisfyAll(
				HaveField("Driver", "ceph"),
				HaveField("Attributes", SatisfyAll(
					HaveKeyWithValue("monitors", cephMonitors),
				)),
				HaveField("SecretData", SatisfyAll(
					HaveKeyWithValue("userID", []byte(cephUsername)),
					HaveKeyWithValue("userKey", []byte(cephUserkey)),
				)),
			)),
		))

		By("attaching volume with connection details to the machine")
		attachVolumeResp, err := machineClient.AttachVolume(ctx, &machineiriv1alpha1.AttachVolumeRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Volume: &machineiriv1alpha1.Volume{
				Name:   listVolumeResp.Volumes[0].Metadata.Id,
				Device: "oda",
				Connection: &machineiriv1alpha1.VolumeConnection{
					Driver: "ceph",
					Handle: listVolumeResp.Volumes[0].Status.Access.Handle,
					Attributes: map[string]string{
						"image":    listVolumeResp.Volumes[0].Status.Access.Attributes["image"],
						"monitors": cephMonitors,
					},
					SecretData: map[string][]byte{
						"userID":  []byte(cephUsername),
						"userKey": []byte(cephUserkey),
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(attachVolumeResp).NotTo(BeNil())

		By("ensuring volume is attached to the machine domain")
		var disks []libvirtxml.DomainDisk
		Eventually(func() int {
			domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
			Expect(err).NotTo(HaveOccurred())
			domainXML := &libvirtxml.Domain{}
			Expect(domainXML.Unmarshal(domainXMLData)).Should(Succeed())
			disks = domainXML.Devices.Disks
			return len(disks)
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Equal(1))
		Expect(disks[0].Serial).To(HavePrefix("oda"))

		By("ensuring attached volume has been updated in machine status field")
		Eventually(func() *machineiriv1alpha1.MachineStatus {
			listResp, err := machineClient.ListMachines(ctx, &machineiriv1alpha1.ListMachinesRequest{
				Filter: &machineiriv1alpha1.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).NotTo(BeEmpty())
			Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status
		}).Should(SatisfyAll(
			HaveField("Volumes", ContainElements(
				&machineiriv1alpha1.VolumeStatus{
					Name:   listVolumeResp.Volumes[0].Metadata.Id,
					Handle: "libvirt-provider.ironcore.dev/ceph/libvirt-provider.ironcore.dev/ceph^" + listVolumeResp.Volumes[0].Status.Access.Handle,
					State:  machineiriv1alpha1.VolumeState_VOLUME_ATTACHED,
				})),
			HaveField("State", Equal(machineiriv1alpha1.MachineState_MACHINE_RUNNING)),
		))
	})

	It("should correctly attach an encrypted volume to the machine", func(ctx SpecContext) {
		By("creating a machine")
		createResp, err := machineClient.CreateMachine(ctx, &machineiriv1alpha1.CreateMachineRequest{
			Machine: &machineiriv1alpha1.Machine{
				Metadata: &irimetav1alpha1.ObjectMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: &machineiriv1alpha1.MachineSpec{
					Power: machineiriv1alpha1.Power_POWER_ON,
					Class: machineClassx2medium,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

		DeferCleanup(func(ctx SpecContext) {
			Eventually(func() bool {
				_, err := machineClient.DeleteMachine(ctx, &machineiriv1alpha1.DeleteMachineRequest{MachineId: createResp.Machine.Metadata.Id})
				Expect(err).To(SatisfyAny(
					BeNil(),
					MatchError(ContainSubstring("NotFound")),
				))
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

		By("creating a volume with encryption key")
		createVolResp, err := volumeClient.CreateVolume(ctx, &volumeiriv1alpha1.CreateVolumeRequest{
			Volume: &volumeiriv1alpha1.Volume{
				Metadata: &irimetav1alpha1.ObjectMetadata{
					Id: "encr-volume",
				},
				Spec: &volumeiriv1alpha1.VolumeSpec{
					Class: "foo",
					Resources: &volumeiriv1alpha1.VolumeResources{
						StorageBytes: 500 * 1024 * 1024,
					},
					Encryption: &volumeiriv1alpha1.EncryptionSpec{
						SecretData: map[string][]byte{
							"encryptionKey": []byte("1cdd0644dce5571c4f31f51b3b1872f5475cb4e3671eafc8e204faf817106895"),
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		By("ensuring the correct encrypted volume is created")
		Expect(createVolResp).Should(SatisfyAll(
			HaveField("Volume.Metadata.Id", Not(BeEmpty())),
			HaveField("Volume.Spec.Class", Equal("foo")),
			HaveField("Volume.Spec.Resources.StorageBytes", Equal(int64(500*1024*1024))),
			HaveField("Volume.Spec.Encryption", BeNil()),
			HaveField("Volume.Status.State", Equal(volumeiriv1alpha1.VolumeState_VOLUME_PENDING)),
			HaveField("Volume.Status.Access", BeNil()),
		))

		DeferCleanup(func(ctx SpecContext) {
			Eventually(func() {
				_, err = volumeClient.DeleteVolume(ctx, &volumeiriv1alpha1.DeleteVolumeRequest{
					VolumeId: createVolResp.Volume.Metadata.Id,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		var listVolumeResp *volumeiriv1alpha1.ListVolumesResponse
		By("ensuring volume is in available state and other status fields have been updated")
		Eventually(func() *volumeiriv1alpha1.VolumeStatus {
			listVolumeResp, err = volumeClient.ListVolumes(ctx, &volumeiriv1alpha1.ListVolumesRequest{
				Filter: &volumeiriv1alpha1.VolumeFilter{
					Id: createVolResp.Volume.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listVolumeResp.Volumes).NotTo(BeEmpty())
			return listVolumeResp.Volumes[0].Status
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(SatisfyAll(
			HaveField("State", Equal(volumeiriv1alpha1.VolumeState_VOLUME_AVAILABLE)),
			HaveField("Access", SatisfyAll(
				HaveField("Driver", "ceph"),
				HaveField("Attributes", SatisfyAll(
					HaveKeyWithValue("monitors", cephMonitors),
				)),
				HaveField("SecretData", SatisfyAll(
					HaveKeyWithValue("userID", []byte(cephUsername)),
					HaveKeyWithValue("userKey", []byte(cephUserkey)),
				)),
			)),
		))

		By("attaching volume with connection details to the machine")
		attachVolumeConnectionResp, err := machineClient.AttachVolume(ctx, &machineiriv1alpha1.AttachVolumeRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Volume: &machineiriv1alpha1.Volume{
				Name:   listVolumeResp.Volumes[0].Metadata.Id,
				Device: "oda",
				Connection: &machineiriv1alpha1.VolumeConnection{
					Driver: "ceph",
					Handle: listVolumeResp.Volumes[0].Status.Access.Handle,
					Attributes: map[string]string{
						"image":    listVolumeResp.Volumes[0].Status.Access.Attributes["image"],
						"monitors": cephMonitors,
					},
					SecretData: map[string][]byte{
						"userID":  []byte(cephUsername),
						"userKey": []byte(cephUserkey),
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(attachVolumeConnectionResp).NotTo(BeNil())

		By("ensuring volume attached to the machine domain")
		var disks []libvirtxml.DomainDisk
		Eventually(func() int {
			domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
			Expect(err).NotTo(HaveOccurred())
			domainXML := &libvirtxml.Domain{}
			Expect(domainXML.Unmarshal(domainXMLData)).Should(Succeed())
			disks = domainXML.Devices.Disks
			return len(disks)
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Equal(1))
		Expect(disks[0].Serial).To(HavePrefix("oda"))

		By("ensuring attached volume has been updated in machine status field")
		Eventually(func() *machineiriv1alpha1.MachineStatus {
			listResp, err := machineClient.ListMachines(ctx, &machineiriv1alpha1.ListMachinesRequest{
				Filter: &machineiriv1alpha1.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Machines).NotTo(BeEmpty())
			Expect(listResp.Machines).Should(HaveLen(1))
			return listResp.Machines[0].Status
		}).Should(SatisfyAll(
			HaveField("Volumes", ContainElements(
				&machineiriv1alpha1.VolumeStatus{
					Name:   listVolumeResp.Volumes[0].Metadata.Id,
					Handle: "libvirt-provider.ironcore.dev/ceph/libvirt-provider.ironcore.dev/ceph^" + listVolumeResp.Volumes[0].Status.Access.Handle,
					State:  machineiriv1alpha1.VolumeState_VOLUME_ATTACHED,
				})),
			HaveField("State", Equal(machineiriv1alpha1.MachineState_MACHINE_RUNNING)),
		))
	})
})
