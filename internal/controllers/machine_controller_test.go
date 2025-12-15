// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers_test

import (
	"github.com/digitalocean/go-libvirt"
	"github.com/ironcore-dev/libvirt-provider/api"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MachineController", func() {
	Context("Machine Lifecycle", func() {
		var machineID string

		It("should create and reconcile a machine", func(ctx SpecContext) {
			By("creating a machine in the store")
			machine, err := createMachine(api.MachineSpec{
				Power:       api.PowerStatePowerOn,
				Cpu:         4,
				MemoryBytes: 8589934592, // 8GB
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(machine).NotTo(BeNil())
			Expect(machine.ID).NotTo(BeEmpty())

			GinkgoWriter.Printf("Created machine: ID=%s\n", machineID)

			DeferCleanup(cleanupMachine(machine.ID))

			By("ensuring domain and domain XML is created for machine")
			var domain libvirt.Domain
			Eventually(func() error {
				domain, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machine.ID))
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

			By("ensuring machine is in running state")
			Eventually(func(g Gomega) {
				m, err := getMachine(machine.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(m.Status.State).To(Equal(api.MachineStateRunning))
			}).Should(Succeed())

		})

		It("should handle machine without boot image, but with volume and network interface", func(ctx SpecContext) {
			By("creating a machine with volume and network interface")
			machine, err := createMachine(api.MachineSpec{
				Power:       api.PowerStatePowerOn,
				Cpu:         2,
				MemoryBytes: 2147483648, // 2GB
				Volumes: []*api.VolumeSpec{
					{
						Name: "disk-1",
						LocalDisk: &api.LocalDiskSpec{
							Size: emptyDiskSize,
						},
						Device: "oda",
					},
				},
				NetworkInterfaces: []*api.NetworkInterfaceSpec{
					{
						Name: "nic-1",
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(machine).NotTo(BeNil())

			GinkgoWriter.Printf("Created machine with NIC: ID=%s\n", machine.ID)

			By("verifying network interface is configured")
			Eventually(func(g Gomega) {
				m, err := getMachine(machine.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(m.Spec.NetworkInterfaces).To(HaveLen(1))
				g.Expect(m.Spec.NetworkInterfaces[0].Name).To(Equal("nic-1"))
			}).Should(Succeed())

			DeferCleanup(cleanupMachine(machine.ID))

			By("ensuring domain and domain XML is created for machine")
			var domain libvirt.Domain
			Eventually(func() error {
				domain, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machine.ID))
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

			By("ensuring machine is in running state and other status fields have been updated")
			Eventually(func(g Gomega) *api.MachineStatus {
				m, err := getMachine(machine.ID)
				g.Expect(err).NotTo(HaveOccurred())
				return &m.Status
			}).Should(SatisfyAll(
				HaveField("ImageRef", BeEmpty()),
				HaveField("VolumeStatus", ContainElement(api.VolumeStatus{
					Name:   "disk-1",
					Handle: "libvirt-provider.ironcore.dev/local-disk/disk-1",
					State:  api.VolumeStateAttached,
					Size:   emptyDiskSize,
				})),
				HaveField("NetworkInterfaceStatus", ContainElement(api.NetworkInterfaceStatus{
					Name:  "nic-1",
					State: api.NetworkInterfaceStateAttached,
				})),
				HaveField("State", Equal(api.MachineStateRunning)),
			))
		})

		It("should update machine power state", func(ctx SpecContext) {
			By("creating a machine in powered on state")
			machine, err := createMachine(api.MachineSpec{
				Power:       api.PowerStatePowerOn,
				Cpu:         2,
				MemoryBytes: 2147483648,
			})
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(cleanupMachine(machine.ID))

			By("ensuring domain and domain XML is created for machine")
			var domain libvirt.Domain
			Eventually(func() error {
				domain, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(machine.ID))
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

			By("ensuring machine is in running state")
			Eventually(func(g Gomega) {
				m, err := getMachine(machine.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(m.Status.State).To(Equal(api.MachineStateRunning))
			}).Should(Succeed())

			By("updating machine to powered off state")
			machine, err = getMachine(machine.ID)
			Expect(err).NotTo(HaveOccurred())
			machine.Spec.Power = api.PowerStatePowerOff
			updatedMachine, err := updateMachine(machine)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedMachine.Spec.Power).To(Equal(api.PowerStatePowerOff))

			By("verifying power state change is persisted")
			Eventually(func(g Gomega) {
				m, err := getMachine(machine.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(m.Spec.Power).To(Equal(api.PowerStatePowerOff))
			}).Should(Succeed())
		})
	})
})
