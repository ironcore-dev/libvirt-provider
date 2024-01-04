// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"net/url"

	"github.com/digitalocean/go-libvirt"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exec", func() {

	It("should return an exec-url with a token", func(ctx SpecContext) {
		By("creating the test machine")
		createResp, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{},
				Spec: &iri.MachineSpec{
					Class: machineClassx3xlarge,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(createResp).NotTo(BeNil())

		DeferCleanup(func(ctx SpecContext) {
			_, err := machineClient.DeleteMachine(ctx, &iri.DeleteMachineRequest{MachineId: createResp.Machine.Metadata.Id})
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
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

		By("issuing exec for the test machine")
		resExec, err := machineClient.Exec(ctx, &iri.ExecRequest{MachineId: createResp.Machine.Metadata.Id})
		Expect(err).NotTo(HaveOccurred())

		By("inspecting the result")
		parsedResUrl, err := url.ParseRequestURI(resExec.Url)
		Expect(err).NotTo(HaveOccurred(), "url is invalid: %q", resExec.Url)
		parsedBaseURL, err := url.ParseRequestURI(baseURL)
		Expect(err).NotTo(HaveOccurred(), "baseUrl is invalid: %q", baseURL)

		Expect(parsedResUrl.Host).To(Equal(parsedBaseURL.Host))
		Expect(parsedResUrl.Scheme).To(Equal(parsedBaseURL.Scheme))
		Expect(parsedResUrl.Path).To(MatchRegexp(`/exec/[^/?&]{8}`))
	})
})
