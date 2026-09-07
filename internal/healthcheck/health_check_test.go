// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package healthcheck_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apiserver/pkg/server/healthz"

	. "github.com/ironcore-dev/libvirt-provider/internal/healthcheck"
)

// stubConnector implements libvirtutils.Connector for tests.
type stubConnector struct {
	connected bool
}

func (c *stubConnector) IsConnected() bool { return c.connected }

var _ = Describe("HealthCheckHandler", func() {
	var (
		h   HealthCheck
		rec *httptest.ResponseRecorder
	)

	newRequest := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/healthz", nil)
	}

	Context("when libvirt and all checkers are healthy", func() {
		BeforeEach(func() {
			h = HealthCheck{
				Libvirt: &stubConnector{connected: true},
				Checkers: []healthz.HealthChecker{
					healthz.NamedCheck("rotator", func(*http.Request) error { return nil }),
				},
			}
		})

		It("reports healthy", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Context("when a named checker fails", func() {
		BeforeEach(func() {
			h = HealthCheck{
				Libvirt: &stubConnector{connected: true},
				Checkers: []healthz.HealthChecker{
					healthz.NamedCheck("apinet-config", func(*http.Request) error {
						return errors.New("certificate is expired")
					}),
				},
			}
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
			h = HealthCheck{
				Libvirt: &stubConnector{connected: true},
				Checkers: []healthz.HealthChecker{
					healthz.NamedCheck("ok-1", func(*http.Request) error { return nil }),
					healthz.NamedCheck("bad", func(*http.Request) error { return errors.New("boom") }),
					healthz.NamedCheck("ok-2", func(*http.Request) error { return nil }),
				},
			}
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

	Context("when libvirt is down but checkers are healthy", func() {
		BeforeEach(func() {
			h = HealthCheck{
				Libvirt: &stubConnector{connected: false},
				Checkers: []healthz.HealthChecker{
					healthz.NamedCheck("rotator", func(*http.Request) error { return nil }),
				},
			}
		})

		It("reports unhealthy", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})

		It("reports the failing libvirt connection in the body", func() {
			rec = httptest.NewRecorder()
			h.HealthCheckHandler(rec, newRequest())
			Expect(rec.Body.String()).To(ContainSubstring("libvirt"))
			Expect(rec.Body.String()).NotTo(ContainSubstring("rotator"))
		})
	})
})
