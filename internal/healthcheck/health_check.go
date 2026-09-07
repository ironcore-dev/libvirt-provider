// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package healthcheck

import (
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"k8s.io/apiserver/pkg/server/healthz"
)

// HealthCheck serves an HTTP handler that reports healthy only when all its
// checkers pass.
type HealthCheck struct {
	log logr.Logger

	// checkers must all pass for the handler to report healthy.
	checkers []healthz.HealthChecker
}

// New returns a HealthCheck serving the given checkers. Any nil checkers (e.g.
// optional controllers that were never started) are filtered out, so the
// handler never has to guard against them.
func New(log logr.Logger, checkers ...healthz.HealthChecker) *HealthCheck {
	c := make([]healthz.HealthChecker, 0, len(checkers))
	for _, checker := range checkers {
		if checker != nil {
			c = append(c, checker)
		}
	}
	return &HealthCheck{
		log:      log,
		checkers: c,
	}
}

func (h HealthCheck) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	for _, checker := range h.checkers {
		if err := checker.Check(r); err != nil {
			msg := fmt.Sprintf("health check failed, check: %s", checker.Name())
			h.log.Error(err, msg)
			http.Error(w, msg, http.StatusServiceUnavailable)

			return
		}
	}

	w.WriteHeader(http.StatusOK)
}
