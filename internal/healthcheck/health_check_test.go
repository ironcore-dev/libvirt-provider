// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package healthcheck_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apiserver/pkg/server/healthz"

	. "github.com/ironcore-dev/libvirt-provider/internal/healthcheck"
)

var _ = Describe("HealthCheckHandler", func() {
	var (
		h   *HealthCheck
		rec *httptest.ResponseRecorder
	)

	newRequest := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/healthz", nil)
	}

	Context("when all checkers are healthy", func() {
		BeforeEach(func() {
			h = New(
				logr.Discard(),
				healthz.NamedCheck("rotator", func(*http.Request) error { return nil }),
			)
		})

		It("reports healthy", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Context("when a named checker fails", func() {
		BeforeEach(func() {
			h = New(
				logr.Discard(),
				healthz.NamedCheck("apinet-config", func(*http.Request) error {
					return errors.New("certificate is expired")
				}),
			)
		})

		It("reports unhealthy", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})

		It("reports the failing checker in the body", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Body.String()).To(ContainSubstring("apinet-config"))
		})
	})

	Context("when only one of multiple checkers fails", func() {
		BeforeEach(func() {
			h = New(
				logr.Discard(),
				healthz.NamedCheck("ok-1", func(*http.Request) error { return nil }),
				healthz.NamedCheck("bad", func(*http.Request) error { return errors.New("boom") }),
				healthz.NamedCheck("ok-2", func(*http.Request) error { return nil }),
			)
		})

		It("still reports unhealthy", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})

		It("reports only the failing checker in the body", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Body.String()).To(ContainSubstring("bad"))
			Expect(rec.Body.String()).NotTo(ContainSubstring("ok-1"))
			Expect(rec.Body.String()).NotTo(ContainSubstring("ok-2"))
		})
	})

	Context("constructed with New", func() {
		It("filters out nil checkers so the handler does not need to guard", func() {
			h := New(
				logr.Discard(),
				// A nil checker (e.g. an optional controller that was never
				// started) must not make the handler panic or require a guard.
				nil,
				healthz.NamedCheck("rotator", func(*http.Request) error { return nil }),
			)

			rec = httptest.NewRecorder()
			Expect(func() { h.HealthCheckHandler(rec, newRequest()) }).NotTo(Panic())
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Context("when the libvirt connection is down but other checkers are healthy", func() {
		BeforeEach(func() {
			h = New(
				logr.Discard(),
				NewLibvirtChecker(&stubConnector{connected: false}),
				healthz.NamedCheck("rotator", func(*http.Request) error { return nil }),
			)
		})

		It("reports unhealthy", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})

		It("reports the failing libvirt connection in the body", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Body.String()).To(ContainSubstring("libvirt connection"))
			Expect(rec.Body.String()).NotTo(ContainSubstring("rotator"))
		})
	})
})
