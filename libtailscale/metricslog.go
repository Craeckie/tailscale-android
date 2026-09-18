// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package libtailscale

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/tailscale/tailscale-android/libtailscale/metricslog"
	"tailscale.com/ipn/ipnlocal"
)

// metricsTickInterval is how often startMetricsLog writes a delta line.
const metricsTickInterval = 10 * time.Minute

var metricsLogger atomic.Pointer[metricslog.Logger]

// startMetricsLog starts logging one metricslog delta line every
// metricsTickInterval for the lifetime of the process.
func startMetricsLog(lb *ipnlocal.LocalBackend) {
	l := metricslog.New(lb, log.Printf)
	metricsLogger.Store(l)
	go l.Run(context.Background(), metricsTickInterval)
}

// MetricsTick logs one metricslog delta line immediately, tagged as a
// power-state transition. Gomobile binds this as Libtailscale.metricsTick();
// it is a no-op if called before startMetricsLog has run.
func MetricsTick() {
	if l := metricsLogger.Load(); l != nil {
		l.Tick("power")
	}
}
