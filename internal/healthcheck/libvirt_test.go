// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package healthcheck_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/ironcore-dev/libvirt-provider/internal/healthcheck"
)

// stubConnector implements libvirtutils.Connector for tests.
type stubConnector struct {
	connected bool
}

func (c *stubConnector) IsConnected() bool { return c.connected }

var _ = Describe("NewLibvirtChecker", func() {
	It("is named libvirt connection", func() {
		checker := NewLibvirtChecker(&stubConnector{connected: true})
		Expect(checker.Name()).To(Equal("libvirt connection"))
	})

	It("reports healthy when connected", func() {
		checker := NewLibvirtChecker(&stubConnector{connected: true})
		Expect(checker.Check(&http.Request{})).To(Succeed())
	})

	It("reports unhealthy when not connected", func() {
		checker := NewLibvirtChecker(&stubConnector{connected: false})
		Expect(checker.Check(&http.Request{})).To(MatchError(ContainSubstring("libvirtd")))
	})
})
