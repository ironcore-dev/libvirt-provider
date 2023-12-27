// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"net/url"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exec", func() {

	It("should return an exec-url with a token", func(ctx SpecContext) {
		By("creating the test machine")
		resCreate, err := machineClient.CreateMachine(ctx, &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{},
				Spec: &iri.MachineSpec{
					Class: machineClassx3xlarge,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resCreate).NotTo(BeNil())
		machineID := resCreate.GetMachine().Metadata.Id

		By("issuing exec for the test machine")
		resExec, err := machineClient.Exec(ctx, &iri.ExecRequest{MachineId: machineID})
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
