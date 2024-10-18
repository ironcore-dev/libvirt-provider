// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/controllers"
	"github.com/ironcore-dev/libvirt-provider/internal/metrics"
)

type ReconcilerMetricsOptions struct {
	ReconcileDuration                prometheus.Observer
	ControllerRuntimeActiveWorker    prometheus.Gauge
	ControllerRuntimeReconcileErrors prometheus.Counter
}

func (r *ReconcilerMetricsOptions) SetMachineReconcilerMetricsOptions() {
	r.ReconcileDuration = metrics.ControllerRuntimeReconcileDuration.WithLabelValues(controllers.MachineReconcilerName)
	r.ControllerRuntimeActiveWorker = metrics.ControllerRuntimeActiveWorker.WithLabelValues(controllers.MachineReconcilerName)
	r.ControllerRuntimeReconcileErrors = metrics.ControllerRuntimeReconcileErrors.WithLabelValues(controllers.MachineReconcilerName)
}

type GuestAgentOption api.GuestAgent

func (g *GuestAgentOption) String() string {
	return string(g.GetAPIGuestAgent())
}

func (g *GuestAgentOption) Set(value string) error {
	if g == nil {
		return fmt.Errorf("invalid pointer to object type %s", g.Type())
	}

	options := guestAgentOptionAvailable()
	index := slices.Index(options, value)
	if index == -1 {
		return fmt.Errorf("unsupported option %s", value)
	}

	*g = GuestAgentOption(value)
	return nil
}

func (g *GuestAgentOption) Type() string {
	return reflect.TypeOf(*g).String()
}

func (g *GuestAgentOption) GetAPIGuestAgent() api.GuestAgent {
	if g == nil || *g == "" {
		return api.GuestAgentNone
	}
	return api.GuestAgent(*g)
}

func guestAgentOptionAvailable() []string {
	return []string{string(api.GuestAgentNone), string(api.GuestAgentQemu)}
}
