// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package wedgewatch

import (
	"context"
	"net/netip"
	"testing"

	"tailscale.com/health"
	"tailscale.com/net/dns/resolver"
	"tailscale.com/net/netmon"
	"tailscale.com/net/tsaddr"
	"tailscale.com/net/tsdial"
	"tailscale.com/util/eventbus/eventbustest"
)

// TestRealResolverAnswersProbe feeds the probe's query to tailscale's own
// quad-100 resolver with an empty config (no netmap, no upstreams) and
// checks that checkAnswer accepts the reply. It fails if a core upgrade
// stops answering ProbeName locally, which would make every probe on the
// phone fail and the watchdog report false wedges.
func TestRealResolverAnswersProbe(t *testing.T) {
	bus := eventbustest.NewBus(t)
	dialer := tsdial.NewDialer(netmon.NewStatic())
	dialer.SetBus(bus)
	r := resolver.New(t.Logf, nil, dialer, health.NewTracker(bus), nil)
	defer r.Close()

	const id = 0x4242
	q, err := buildQuery(id)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.Query(context.Background(), q, "udp", netip.MustParseAddrPort("100.64.0.1:12345"))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	ok, err := checkAnswer(resp, id, tsaddr.TailscaleServiceIP())
	if !ok || err != nil {
		t.Fatalf("checkAnswer = %v, %v; response % x", ok, err, resp)
	}
}
