// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/digitalocean/go-libvirt"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/util/term"
)

var _ = Describe("Exec", func() {

	It("should verify an exec-url with a token", func(ctx SpecContext) {
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
		Eventually(func(g Gomega) libvirt.DomainState {
			domainState, _, err := libvirtConn.DomainGetState(domain, 0)
			g.Expect(err).NotTo(HaveOccurred())
			return libvirt.DomainState(domainState)
		}).Should(Equal(libvirt.DomainRunning))

		By("getting exec-url for the test machine")
		execResp, err := machineClient.Exec(ctx, &iri.ExecRequest{MachineId: createResp.Machine.Metadata.Id})
		Expect(err).NotTo(HaveOccurred())

		By("inspecting the result")
		parsedResUrl, err := url.ParseRequestURI(execResp.Url)
		Expect(err).NotTo(HaveOccurred(), "url is invalid: %q", execResp.Url)
		parsedBaseURL, err := url.ParseRequestURI(baseURL)
		Expect(err).NotTo(HaveOccurred(), "baseUrl is invalid: %q", baseURL)

		Expect(parsedResUrl.Host).To(Equal(parsedBaseURL.Host))
		Expect(parsedResUrl.Scheme).To(Equal(parsedBaseURL.Scheme))
		Expect(parsedResUrl.Path).To(MatchRegexp(`/exec/[^/?&]{8}`))

		By("issuing exec with response URL received and verifying tty stream")
		err = runExec(ctx, parsedResUrl)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying same token cannot be used twice")
		err = runExec(ctx, parsedResUrl)
		Expect(err).To(MatchError(ContainSubstring("404 page not found")), "Rejecting unknown / expired token")

		By("getting exec-url with new token")
		execResp, err = machineClient.Exec(ctx, &iri.ExecRequest{MachineId: createResp.Machine.Metadata.Id})
		Expect(err).NotTo(HaveOccurred())

		By("inspecting the result")
		parsedResUrl, err = url.ParseRequestURI(execResp.Url)
		Expect(err).NotTo(HaveOccurred(), "url is invalid: %q", execResp.Url)

		By("deleting the machine")
		_, err = machineClient.DeleteMachine(ctx, &iri.DeleteMachineRequest{MachineId: createResp.Machine.Metadata.Id})
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(func() bool {
			_, err = libvirtConn.DomainLookupByUUID(libvirtutils.UUIDStringToBytes(createResp.Machine.Metadata.Id))
			return libvirt.IsNotFound(err)
		}).Should(BeTrue())

		By("verifying exec url with a valid token and a not existing machine fails")
		err = runExec(ctx, parsedResUrl)
		machineNotSynchErr := fmt.Sprintf("machine %s has not yet been synced", createResp.Machine.Metadata.Id)
		Expect(err).To(SatisfyAny(MatchError(ContainSubstring("404 page not found")), MatchError(ContainSubstring(machineNotSynchErr))))

	})
})

func runExec(ctx context.Context, execUrl *url.URL) error {
	randomSize := 1024 * 1024
	randomData := make([]byte, randomSize)
	var stdout bytes.Buffer

	tty := term.TTY{
		In:     bytes.NewReader(randomData),
		Out:    &stdout,
		Raw:    true,
		TryDev: true,
	}

	roundTripper, err := spdy.NewRoundTripperWithConfig(spdy.RoundTripperConfig{
		TLS:        http.DefaultTransport.(*http.Transport).TLSClientConfig,
		Proxier:    http.ProxyFromEnvironment,
		PingPeriod: 5 * time.Second,
	})
	if err != nil {
		return err
	}
	exec, err := remotecommand.NewSPDYExecutorForTransports(roundTripper, roundTripper, http.MethodGet, execUrl)
	if err != nil {
		return err
	}

	return tty.Safe(func() error {
		return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  tty.In,
			Stdout: tty.Out,
			Tty:    true,
		})
	})
}
