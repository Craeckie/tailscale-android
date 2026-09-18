// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package metricslog

import (
	"fmt"
	"strings"
	"testing"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

type fakeSource struct {
	st   *ipnstate.Status
	exit string
}

func (f fakeSource) Status() *ipnstate.Status { return f.st }
func (f fakeSource) Prefs() ipn.PrefsView {
	return (&ipn.Prefs{ExitNodeID: tailcfg.StableNodeID(f.exit)}).View()
}

// newLoggerAt returns a Logger whose next Tick computes its delta from prev
// to cur (cur is returned on every subsequent metrics() call too, so a
// second Tick in the same test would see a zero delta).
func newLoggerAt(src Source, prev, cur map[string]int64) (*Logger, *[]string) {
	var lines []string
	l := &Logger{
		src:     src,
		logf:    func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) },
		metrics: func() map[string]int64 { return cur },
		prev:    prev,
	}
	return l, &lines
}

func TestTickDeltasAndAbsoluteFields(t *testing.T) {
	peerA := key.NewNode().Public()
	peerB := key.NewNode().Public()
	st := &ipnstate.Status{
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			peerA: {TxBytes: 100, RxBytes: 200, ExitNode: true},
			peerB: {TxBytes: 5, RxBytes: 5},
		},
	}
	src := fakeSource{st: st, exit: "nExit123"}

	prev := map[string]int64{
		"netcheck_report":            1,
		"netcheck_stun_send_ipv4":    2,
		"netcheck_stun_send_ipv6":    1,
		"magicsock_send_udp":         10,
		"magicsock_send_derp":        3,
		"magicsock_disco_sent_ping":  4,
		"magicsock_num_derp_conns":   2,
		"derp_home_change":           0,
		"controlclient_map_requests": 1,
		"dns_query_fwd":              5,
		"dns_query_fwd_doh":          5,
		"tstun_in_from_wg":           100,
		"tstun_out_to_wg":            100,
		"fork_flowlog_new_flows":     1,
	}
	cur := map[string]int64{
		"netcheck_report":            27,
		"netcheck_stun_send_ipv4":    30,
		"netcheck_stun_send_ipv6":    10,
		"magicsock_send_udp":         210,
		"magicsock_send_derp":        13,
		"magicsock_disco_sent_ping":  44,
		"magicsock_num_derp_conns":   3,
		"derp_home_change":           1,
		"controlclient_map_requests": 2,
		"dns_query_fwd":              25,
		"dns_query_fwd_doh":          20,
		"tstun_in_from_wg":           400,
		"tstun_out_to_wg":            500,
		"fork_flowlog_new_flows":     6,
	}

	l, lines := newLoggerAt(src, prev, cur)
	l.Tick("tick")
	if len(*lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(*lines))
	}
	got := (*lines)[0]

	want := "metrics: exit_node=nExit123 netcheck=26 stun_tx=37 udp_tx=200 derp_tx=10 disco_ping=40 derp_conns=3 derp_home=1 map_req=1 dns_fwd=20 dns_doh=15 tun_in=300 tun_out=400 flows=5 peers=" +
		peerA.ShortString() + "*:100/200," + peerB.ShortString() + ":5/5 why=tick"
	if got != want {
		t.Errorf("Tick line:\n got  %q\nwant %q", got, want)
	}
}

func TestTickNoExitNode(t *testing.T) {
	src := fakeSource{st: &ipnstate.Status{}, exit: ""}
	l, lines := newLoggerAt(src, map[string]int64{}, map[string]int64{})
	l.Tick("power")
	got := (*lines)[0]
	if !strings.HasPrefix(got, "metrics: exit_node=- ") {
		t.Errorf("exit_node marker: got %q, want prefix \"metrics: exit_node=- \"", got)
	}
	if !strings.HasSuffix(got, "peers= why=power") {
		t.Errorf("why/peers: got %q, want suffix \"peers= why=power\"", got)
	}
}

func TestTickMissingMetricIsZero(t *testing.T) {
	src := fakeSource{st: &ipnstate.Status{}, exit: ""}
	l, lines := newLoggerAt(src, map[string]int64{}, map[string]int64{})
	l.Tick("tick")
	got := (*lines)[0]
	if !strings.Contains(got, "netcheck=0 stun_tx=0 udp_tx=0 derp_tx=0 disco_ping=0 derp_conns=0 derp_home=0 map_req=0 dns_fwd=0 dns_doh=0 tun_in=0 tun_out=0 flows=0") {
		t.Errorf("missing metrics should read 0: got %q", got)
	}
}

func TestFormatPeersSortsAndCaps(t *testing.T) {
	peers := map[key.NodePublic]*ipnstate.PeerStatus{}
	var keys []key.NodePublic
	for i := 0; i < maxPeers+3; i++ {
		k := key.NewNode().Public()
		keys = append(keys, k)
		peers[k] = &ipnstate.PeerStatus{TxBytes: int64(i + 1), RxBytes: 0}
	}
	// A zero-traffic peer must be excluded entirely.
	zero := key.NewNode().Public()
	peers[zero] = &ipnstate.PeerStatus{}

	got := formatPeers(&ipnstate.Status{Peer: peers})
	entries := strings.Split(got, ",")
	if len(entries) != maxPeers {
		t.Fatalf("got %d peer entries, want %d (cap)", len(entries), maxPeers)
	}
	for _, e := range entries {
		if strings.Contains(e, zero.ShortString()) {
			t.Errorf("zero-traffic peer %s must not appear: %q", zero.ShortString(), got)
		}
	}
	// Busiest peer (highest i) must be first.
	busiest := keys[len(keys)-1]
	if !strings.HasPrefix(entries[0], busiest.ShortString()+":") {
		t.Errorf("first entry %q, want busiest peer %s first", entries[0], busiest.ShortString())
	}
}
