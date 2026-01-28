// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	libvirtserver "github.com/ironcore-dev/libvirt-provider/internal/server"
	claim "github.com/ironcore-dev/provider-utils/claimutils/claim"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	baseURL = "http://localhost:8080"
)

func TestHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Handler Suite")
}

var _ = Describe("HTTP Handler", func() {
	var (
		server *libvirtserver.Server
		router http.Handler
	)
	BeforeEach(func() {
		var err error
		server, err = libvirtserver.New(libvirtserver.Options{
			BaseURL:         baseURL,
			GuestAgent:      api.GuestAgentNone,
			ResourceClaimer: &NOOPResourceClaimer{},
		})
		Expect(err).ShouldNot(HaveOccurred())
		router = NewHandler(server, HandlerOptions{})
	})
	Describe("NewHandler", func() {
		Context("when handling a GET request for unknown token", func() {
			It("should respond Not Found", func() {
				req, err := http.NewRequest(http.MethodGet, "/exec/unknowntoken", nil)
				Expect(err).NotTo(HaveOccurred())
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				Expect(recorder.Code).To(Equal(http.StatusNotFound))
			})
		})
		It("should fail when http request with a not expected method called", func() {
			req, err := http.NewRequest(http.MethodPut, "/exec/unknowntoken", nil)
			Expect(err).NotTo(HaveOccurred())
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			Expect(recorder.Code).To(Equal(http.StatusMethodNotAllowed))
		})
	})
})

type NOOPResourceClaimer struct{}

// Claim implements [claim.Claimer].
func (n *NOOPResourceClaimer) Claim(ctx context.Context, resources v1alpha1.ResourceList) (claim.Claims, error) {
	return nil, nil
}

// Release implements [claim.Claimer].
func (n *NOOPResourceClaimer) Release(ctx context.Context, claims claim.Claims) error {
	return nil
}

// Start implements [claim.Claimer].
func (n *NOOPResourceClaimer) Start(ctx context.Context) error {
	return nil
}

// WaitUntilStarted implements [claim.Claimer].
func (n *NOOPResourceClaimer) WaitUntilStarted(ctx context.Context) error {
	return nil
}
