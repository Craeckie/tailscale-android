// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package libtailscale

import (
	"context"
	"log"
	"net/netip"
	"time"

	"github.com/tailscale/tailscale-android/libtailscale/wedgewatch"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnlocal"
	"tailscale.com/net/tsaddr"
)

// startWedgeWatch starts the quad-100 reply-path watchdog (fork-only, see
// package wedgewatch) for the lifetime of the process. It is started from
// startMetricsLog, so it needs no hook of its own in upstream files.
func startWedgeWatch(lb *ipnlocal.LocalBackend) {
	quad100 := tsaddr.TailscaleServiceIP()
	server := netip.AddrPortFrom(quad100, 53)
	w := wedgewatch.New(wedgewatch.Config{
		Interval:  time.Minute,
		Timeout:   5 * time.Second,
		Threshold: 2,
		MaxDumps:  3,
		Logf:      log.Printf,
		Probe: func(ctx context.Context) error {
			// Inside the probe so that a stuck LocalBackend mutex counts as
			// a failed round rather than blocking the watcher.
			if lb.State() != ipn.Running || !lb.Prefs().CorpDNS() {
				return wedgewatch.ErrInactive
			}
			return wedgewatch.QueryA(ctx, server, quad100)
		},
	})
	go w.Run(context.Background())
}
