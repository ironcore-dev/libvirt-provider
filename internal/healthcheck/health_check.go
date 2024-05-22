// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package healthcheck

import (
	"net/http"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
)

type HealthCheck struct {
	Libvirt *libvirt.Libvirt
	Log     logr.Logger
}

func (h HealthCheck) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if err := libvirtutils.IsConnected(h.Libvirt); err == nil {
		w.WriteHeader(http.StatusOK)
	} else {
		h.Log.Error(err, "failed to get active connection to libvirtd")
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}
