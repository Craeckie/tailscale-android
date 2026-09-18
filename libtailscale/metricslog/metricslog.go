// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Package metricslog periodically logs one line of client-metric deltas for
// the counters that matter for battery investigations (netcheck runs,
// STUN/UDP/DERP sends, disco pings, DERP connection churn, DNS forwards, TUN
// packet counts), plus the current exit node and per-peer byte counters, so
// a capture carries the numbers instead of needing grep counts.
package metricslog

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/logger"
	"tailscale.com/util/clientmetric"
)

// maxPeers caps how many peer byte counters a line carries.
const maxPeers = 12

// Source is the subset of *ipnlocal.LocalBackend that Logger needs.
type Source interface {
	Status() *ipnstate.Status
	Prefs() ipn.PrefsView
}

// Logger periodically logs one line of client-metric deltas plus the
// current exit node and per-peer byte counters.
type Logger struct {
	src  Source
	logf logger.Logf

	// metrics returns the current value of every published client metric,
	// keyed by name. Overridden in tests; New sets it to readMetrics.
	metrics func() map[string]int64

	mu   sync.Mutex
	prev map[string]int64 // snapshot as of the last Tick (or New, before the first Tick)
}

// New creates a Logger that reads src and logs via logf. It takes an
// immediate baseline snapshot, so a Tick called before Run has started
// still produces a sensible (zero) delta rather than one since process
// start.
func New(src Source, logf logger.Logf) *Logger {
	l := &Logger{src: src, logf: logf, metrics: readMetrics}
	l.prev = l.metrics()
	return l
}

func readMetrics() map[string]int64 {
	m := make(map[string]int64)
	for _, metric := range clientmetric.Metrics() {
		m[metric.Name()] = metric.Value()
	}
	return m
}

// Run logs one delta line every interval until ctx is done.
func (l *Logger) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.Tick("tick")
		}
	}
}

// Tick logs one delta line now, tagging it with why ("tick" from Run's
// ticker, "power" from a forced call around a power-state transition). The
// ticker is not reset by a forced tick: a "tick" row shortly after a
// "power" row is fine, since deltas still add up across both.
func (l *Logger) Tick(why string) {
	cur := l.metrics()

	l.mu.Lock()
	prev := l.prev
	l.prev = cur
	l.mu.Unlock()

	l.logf("%s", format(l.src.Status(), l.src.Prefs(), prev, cur, why))
}

func format(st *ipnstate.Status, prefs ipn.PrefsView, prev, cur map[string]int64, why string) string {
	d := func(name string) int64 { return cur[name] - prev[name] }

	exitNode := string(prefs.ExitNodeID())
	if exitNode == "" {
		exitNode = "-"
	}

	return fmt.Sprintf(
		"metrics: exit_node=%s netcheck=%d stun_tx=%d udp_tx=%d derp_tx=%d disco_ping=%d derp_conns=%d derp_home=%d map_req=%d dns_fwd=%d dns_doh=%d tun_in=%d tun_out=%d flows=%d peers=%s why=%s",
		exitNode,
		d("netcheck_report"),
		d("netcheck_stun_send_ipv4")+d("netcheck_stun_send_ipv6"),
		d("magicsock_send_udp"),
		d("magicsock_send_derp"),
		d("magicsock_disco_sent_ping"),
		cur["magicsock_num_derp_conns"], // gauge: absolute, not a delta
		d("derp_home_change"),
		d("controlclient_map_requests"),
		d("dns_query_fwd"),
		d("dns_query_fwd_doh"),
		d("tstun_in_from_wg"),
		d("tstun_out_to_wg"),
		d("fork_flowlog_new_flows"),
		formatPeers(st),
		why,
	)
}

type peerBytes struct {
	key    string
	tx, rx int64
	exit   bool
}

// formatPeers renders peers with any traffic, busiest first, capped at
// maxPeers, keyed by the same short form magicsock uses in its own log
// lines so rows correlate. The current exit node peer (if any) is marked
// with a trailing "*". No IPs or hostnames, so the redactor has nothing to
// do with this line.
func formatPeers(st *ipnstate.Status) string {
	if st == nil {
		return ""
	}
	peers := make([]peerBytes, 0, len(st.Peer))
	for pub, ps := range st.Peer {
		if ps == nil {
			continue
		}
		if ps.TxBytes <= 0 && ps.RxBytes <= 0 {
			continue
		}
		peers = append(peers, peerBytes{pub.ShortString(), ps.TxBytes, ps.RxBytes, ps.ExitNode})
	}
	sort.Slice(peers, func(i, j int) bool {
		ti, tj := peers[i].tx+peers[i].rx, peers[j].tx+peers[j].rx
		if ti != tj {
			return ti > tj
		}
		return peers[i].key < peers[j].key
	})
	if len(peers) > maxPeers {
		peers = peers[:maxPeers]
	}
	parts := make([]string, len(peers))
	for i, p := range peers {
		marker := ""
		if p.exit {
			marker = "*"
		}
		parts[i] = fmt.Sprintf("%s%s:%d/%d", p.key, marker, p.tx, p.rx)
	}
	return strings.Join(parts, ",")
}
