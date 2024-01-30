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

var _ = Describe("DetachNetworkInterface", func() {
	It("should detach a network interface from the machine", func(ctx SpecContext) {
		By("creating a machine")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: &iri.MachineSpec{
					Power: iri.Power_POWER_ON,
					Class: machineClassx2medium,
					Image: &iri.ImageSpec{
						Image: osImage,
					},
					NetworkInterfaces: []*iri.NetworkInterface{
						{
							Name:      "nic-1",
							NetworkId: "nid-1",
							Ips:       []string{"192.168.1.1"},
							Attributes: map[string]string{
								"key1": "value1",
							},
						},
						{
							Name:      "nic-2",
							NetworkId: "nid-2",
							Ips:       []string{"192.168.1.2"},
							Attributes: map[string]string{
								"key2": "value2",
							},
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

		By("ensuring both network interfaces are attached to the machine domain")
		var interfaces []libvirtxml.DomainInterface
		Eventually(func() int {
			domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
			Expect(err).NotTo(HaveOccurred())
			domainXML := &libvirtxml.Domain{}
			err = domainXML.Unmarshal(domainXMLData)
			Expect(err).NotTo(HaveOccurred())
			interfaces = domainXML.Devices.Interfaces
			return len(interfaces)
		}).Should(Equal(2))
		Expect(interfaces[0].Alias.Name).To(HaveSuffix("nic-1"))
		Expect(interfaces[1].Alias.Name).To(HaveSuffix("nic-2"))

		By("detaching nic-1 network interface from the machine")
		detachNetworkResp, err := machineClient.DetachNetworkInterface(ctx, &iri.DetachNetworkInterfaceRequest{
			MachineId: createResp.Machine.Metadata.Id,
			Name:      "nic-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(detachNetworkResp).NotTo(BeNil())

		By("ensuring network interface has been updated in the machine status")
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
			HaveField("NetworkInterfaces", ContainElements(
				&iri.NetworkInterfaceStatus{
					Name:  "nic-2",
					State: iri.NetworkInterfaceState_NETWORK_INTERFACE_ATTACHED,
				})),
			HaveField("State", Equal(iri.MachineState_MACHINE_RUNNING)),
		))
	})
})
