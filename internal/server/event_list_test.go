// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"time"

	"github.com/digitalocean/go-libvirt"
	irievent "github.com/ironcore-dev/ironcore/iri/apis/event/v1alpha1"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	machinepoolletv1alpha1 "github.com/ironcore-dev/ironcore/poollet/machinepoollet/api/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("ListEvents", func() {
	It("should correctly list events", func(ctx SpecContext) {
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
					Class: machineClassx3xlarge,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

		DeferCleanup(func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				_, err := machineClient.DeleteMachine(ctx, &iri.DeleteMachineRequest{MachineId: createResp.Machine.Metadata.Id})
				g.Expect(err).To(SatisfyAny(
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

		By("listing the machine events with no filters")
		resp, err := machineClient.ListEvents(ctx, &iri.ListEventsRequest{})
		Expect(err).NotTo(HaveOccurred())

		Expect(resp.Events).To(ContainElement(
			HaveField("Spec", SatisfyAll(
				HaveField("InvolvedObjectMeta.Id", Equal(createResp.Machine.Metadata.Id)),
				HaveField("Reason", Equal("NoIgnitionData")),
				HaveField("Message", Equal("Machine does not have ignition data")),
				HaveField("Type", Equal(corev1.EventTypeWarning)),
			)),
		))

		By("listing the machine events with matching label and time filters")
		resp, err = machineClient.ListEvents(ctx, &iri.ListEventsRequest{Filter: &iri.EventFilter{
			LabelSelector:  map[string]string{"foo": "bar"},
			EventsFromTime: time.Now().Add(-5 * time.Second).Unix(),
			EventsToTime:   time.Now().Unix(),
		}})

		Expect(err).NotTo(HaveOccurred())

		Expect(resp.Events).To(ContainElement(
			HaveField("Spec", SatisfyAll(
				HaveField("InvolvedObjectMeta.Id", Equal(createResp.Machine.Metadata.Id)),
				HaveField("Reason", Equal("NoIgnitionData")),
				HaveField("Message", Equal("Machine does not have ignition data")),
				HaveField("Type", Equal(corev1.EventTypeWarning)),
			)),
		))

		By("listing the machine events with non matching label filter")
		resp, err = machineClient.ListEvents(ctx, &iri.ListEventsRequest{Filter: &iri.EventFilter{
			LabelSelector:  map[string]string{"incorrect": "label"},
			EventsFromTime: time.Now().Add(-5 * time.Second).Unix(),
			EventsToTime:   time.Now().Unix(),
		}})
		Expect(err).NotTo(HaveOccurred())

		Expect(resp.Events).To(BeEmpty())

		By("listing the machine events with matching label filter and non matching time filter")
		resp, err = machineClient.ListEvents(ctx, &iri.ListEventsRequest{Filter: &iri.EventFilter{
			LabelSelector:  map[string]string{machinepoolletv1alpha1.MachineUIDLabel: "foobar"},
			EventsFromTime: time.Now().Add(-10 * time.Minute).Unix(),
			EventsToTime:   time.Now().Add(-5 * time.Minute).Unix(),
		}})
		Expect(err).NotTo(HaveOccurred())

		Expect(resp.Events).To(BeEmpty())

		By("listing the machine events with expired TTL")
		Eventually(func(g Gomega) []*irievent.Event {
			resp, err := machineClient.ListEvents(ctx, &iri.ListEventsRequest{})
			g.Expect(err).NotTo(HaveOccurred())
			return resp.Events
		}).Should(BeEmpty())
	})
})
