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

var _ = Describe("AttachNetworkInterface", func() {
	It("should attach network interface to machines", func(ctx SpecContext) {
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

		By("attaching network interface to a machine")
		attachNetworkResp, err := machineClient.AttachNetworkInterface(ctx, &iri.AttachNetworkInterfaceRequest{
			MachineId: createResp.Machine.Metadata.Id,
			NetworkInterface: &iri.NetworkInterface{
				Name: "nic-1",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(attachNetworkResp).NotTo(BeNil())

		By("ensuring network interface attached to a machine domain")
		var interfaces []libvirtxml.DomainInterface
		Eventually(func() int {
			domainXMLData, err := libvirtConn.DomainGetXMLDesc(domain, 0)
			Expect(err).NotTo(HaveOccurred())
			domainXML := &libvirtxml.Domain{}
			err = domainXML.Unmarshal(domainXMLData)
			Expect(err).NotTo(HaveOccurred())
			interfaces = domainXML.Devices.Interfaces
			return len(interfaces)
		}).WithTimeout(10 * time.Second).WithPolling(1 * time.Second).Should(Equal(1))
		Expect(interfaces[0].Alias.Name).To(HaveSuffix("nic-1"))

		By("ensuring attached NIC has been updated in machine status field")
		Eventually(func() *iri.MachineStatus {
			resp, err := machineClient.ListMachines(ctx, &iri.ListMachinesRequest{
				Filter: &iri.MachineFilter{
					Id: createResp.Machine.Metadata.Id,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Machines).NotTo(BeEmpty())
			return resp.Machines[0].Status
		}).Should(SatisfyAll(
			HaveField("NetworkInterfaces", ContainElements(
				&iri.NetworkInterfaceStatus{
					Name:  "nic-1",
					State: iri.NetworkInterfaceState_NETWORK_INTERFACE_ATTACHED,
				})),
			HaveField("State", Equal(iri.MachineState_MACHINE_RUNNING)),
		))
	})
})