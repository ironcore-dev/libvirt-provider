// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"net/url"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exec", func() {

	It("should return an exec-url with a token", func(ctx SpecContext) {
		By("creating the test machine")
		resCreate, err := srv.CreateMachine(context.TODO(), &iri.CreateMachineRequest{
			Machine: &iri.Machine{
				Metadata: &irimeta.ObjectMetadata{},
				Spec: &iri.MachineSpec{
					Class: "x3-xlarge",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resCreate).NotTo(BeNil())
		machineID := resCreate.GetMachine().Metadata.Id

		By("issuing exec for the test machine")
		resExec, err := srv.Exec(ctx, &iri.ExecRequest{MachineId: machineID})
		Expect(err).NotTo(HaveOccurred())

		By("inspecting the result")
		u, err := url.ParseRequestURI(resExec.Url)
		Expect(err).NotTo(HaveOccurred(), "url is invalid: %q", resExec.Url)
		Expect(u.Host).To(Equal("localhost:8080"))
		Expect(u.Scheme).To(Equal("http"))
		Expect(u.Path).To(MatchRegexp(`/exec/[^/?&]{8}`))
	})
})
