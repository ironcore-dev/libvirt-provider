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
)

var _ = Describe("Create Machine", func() {

	It("should create a machine", func(ctx SpecContext) {
		ignitionData := []byte("urjhikmnbdjfkknhhdddeee")
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
					Image: &iri.ImageSpec{
						Image: "ghcr.io/ironcore-dev/ironcore-image/gardenlinux:rootfs-dev-20231206-v1",
					},
					Class:        machineClassx3xlarge,
					IgnitionData: ignitionData,
					// TODO: will be done when convertMachineToIRIMachine() supports Volumes and NetworkInterfaces
					Volumes:           nil,
					NetworkInterfaces: nil,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		By("ensuring the correct creation response")
		Expect(createResp).Should(SatisfyAll(
			HaveField("Machine.Metadata.Id", Not(BeEmpty())),
			HaveField("Machine.Spec.Power", iri.Power_POWER_ON),
			HaveField("Machine.Spec.Image", Not(BeNil())),
			HaveField("Machine.Spec.Class", machineClassx3xlarge),
			HaveField("Machine.Spec.IgnitionData", Equal(ignitionData)),
			HaveField("Machine.Spec.Volumes", BeNil()),
			HaveField("Machine.Spec.NetworkInterfaces", BeNil()),
			HaveField("Machine.Status.ObservedGeneration", BeZero()),
			HaveField("Machine.Status.State", Equal(iri.MachineState_MACHINE_PENDING)),
			HaveField("Machine.Status.ImageRef", BeEmpty()),
			HaveField("Machine.Status.Volumes", BeNil()),
			HaveField("Machine.Status.NetworkInterfaces", BeNil()),
		))

		DeferCleanup(machineClient.DeleteMachine, &iri.DeleteMachineRequest{
			MachineId: createResp.Machine.Metadata.Id,
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

		By("updating machine annotations")
		Eventually(func() error {
			_, err := machineClient.UpdateMachineAnnotations(ctx, &iri.UpdateMachineAnnotationsRequest{
				MachineId: createResp.Machine.Metadata.Id,
				Annotations: map[string]string{
					"machinepoolletv1alpha1.MachineUIDLabel": "foobar",
				}})
			return err
		}).Should(Succeed())

		By("ensuring machine is in running state and other status fields have been updated")
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
			HaveField("ObservedGeneration", BeZero()),
			HaveField("ImageRef", BeEmpty()),
			HaveField("Volumes", BeNil()),
			HaveField("NetworkInterfaces", BeNil()),
			HaveField("State", Equal(iri.MachineState_MACHINE_RUNNING)),
		))
	})
})
