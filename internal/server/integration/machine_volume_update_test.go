// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	"time"

	"github.com/digitalocean/go-libvirt"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"libvirt.org/go/libvirtxml"
)

var _ = Describe("UpdateVolume", func() {
	It("should correctly update machine volume", func(ctx SpecContext) {
		By("creating a machine with ceph volume")
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
							Name:   "rootdisk",
							Device: "oda",
							LocalDisk: &iri.LocalDisk{
								Image: &iri.ImageSpec{
									Image: osImage,
								},
							},
						},
						{
							Name:   "volume-1",
							Device: "odb",
							Connection: &iri.VolumeConnection{
								Driver: "ceph",
								Handle: "dummy",
								Attributes: map[string]string{
									"image":    cephImage,
									"monitors": cephMonitors,
								},
								SecretData: map[string][]byte{
									"userID":  []byte(cephUsername),
									"userKey": []byte(cephUserkey),
								},
								EffectiveStorageBytes: resource.NewQuantity(1*1024*1024*1024, resource.BinarySI).Value(),
							},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

		DeferCleanup(cleanupMachine(createResp.Machine.Metadata.Id))

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
		Eventually(func(g Gomega) libvirt.DomainState {
			domainState, _, err := libvirtConn.DomainGetState(domain, 0)
			g.Expect(err).NotTo(HaveOccurred())
			return libvirt.DomainState(domainState)
		}).Should(Equal(libvirt.DomainRunning))

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
			HaveField("Volumes", ContainElements(
				&iri.VolumeStatus{
					Name:   "volume-1",
					Handle: "libvirt-provider.ironcore.dev/ceph/libvirt-provider.ironcore.dev/ceph^dummy",
					State:  iri.VolumeState_VOLUME_ATTACHED,
				})),
			HaveField("State", Equal(iri.MachineState_MACHINE_RUNNING)),
		))

		By("ensuring the ceph volume is attached to a machine domain")
		var disks []libvirtxml.DomainDisk
		Eventually(func(g Gomega) int {
			domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
			g.Expect(err).NotTo(HaveOccurred())
			domainXML := &libvirtxml.Domain{}
			g.Expect(domainXML.Unmarshal(domainXMLData)).Should(Succeed())
			disks = domainXML.Devices.Disks
			return len(disks)
		}).Should(Equal(2))
		Expect(disks[1].Serial).To(HavePrefix("odb"))

		By("updating machine volume")
		updateVolumeResp, err := machineClient.UpdateVolume(ctx, &iri.UpdateVolumeRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Volume: &iri.Volume{
				Name:   "volume-1",
				Device: "odb",
				Connection: &iri.VolumeConnection{
					Driver: "ceph",
					Handle: "dummy",
					Attributes: map[string]string{
						"image":    cephImage,
						"monitors": cephMonitors,
					},
					SecretData: map[string][]byte{
						"userID":  []byte(cephUsername),
						"userKey": []byte(cephUserkey),
					},
					EffectiveStorageBytes: resource.NewQuantity(2*1024*1024*1024, resource.BinarySI).Value(),
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(updateVolumeResp).NotTo(BeNil())

		By("ensuring volume has been resized and updated in machine spec field")
		Eventually(func(g Gomega) *iri.Volume {
			listResp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(listResp.Machines).NotTo(BeEmpty())
			g.Expect(listResp.Machines).Should(HaveLen(1))
			g.Expect(listResp.Machines[0].Spec.Volumes).Should(HaveLen(2))
			return listResp.Machines[0].Spec.Volumes[1]
		}).Should(SatisfyAll(
			HaveField("Name", Equal("volume-1")),
			HaveField("Device", Equal("odb")),
			HaveField("Connection.EffectiveStorageBytes", Equal(resource.NewQuantity(2*1024*1024*1024, resource.BinarySI).Value())),
		))
	})
})
