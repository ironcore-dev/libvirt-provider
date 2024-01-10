// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	libvirtserver "github.com/ironcore-dev/libvirt-provider/provider/server"
	. "github.com/onsi/ginkgo"
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
		server, _ = libvirtserver.New(libvirtserver.Options{
			BaseURL: baseURL,
		})
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
	})
})
