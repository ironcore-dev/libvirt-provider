// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package healthcheck

import (
	"fmt"
	"net/http"

	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	"k8s.io/apiserver/pkg/server/healthz"
)

// NewLibvirtChecker returns a HealthChecker that reports healthy when the
// given libvirt connector has an active connection to libvirtd.
func NewLibvirtChecker(conn libvirtutils.Connector) healthz.HealthChecker {
	return healthz.NamedCheck("libvirt connection", func(*http.Request) error {
		if err := libvirtutils.IsConnected(conn); err != nil {
			return fmt.Errorf("failed to get active connection to libvirtd: %w", err)
		}
		return nil
	})
}
