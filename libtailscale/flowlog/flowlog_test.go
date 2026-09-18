// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package flowlog

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"tailscale.com/net/packet"
	"tailscale.com/net/tstun"
	"tailscale.com/types/ipproto"
	"tailscale.com/wgengine/filter"
)

func newTestLogger(owner OwnerFunc) (*Logger, chan string) {
	lines := make(chan string, 32)
	logf := func(format string, args ...any) {
		lines <- fmt.Sprintf(format, args...)
	}
	return New(logf, owner), lines
}

func tcpSyn(src, dst string) *packet.Parsed {
	return &packet.Parsed{
		IPVersion: 4,
		IPProto:   ipproto.TCP,
		Src:       netip.MustParseAddrPort(src),
		Dst:       netip.MustParseAddrPort(dst),
		TCPFlags:  packet.TCPSyn,
	}
}

func tcpData(src, dst string) *packet.Parsed {
	return &packet.Parsed{
		IPVersion: 4,
		IPProto:   ipproto.TCP,
		Src:       netip.MustParseAddrPort(src),
		Dst:       netip.MustParseAddrPort(dst),
		TCPFlags:  packet.TCPAck,
	}
}

func udpPacket(src, dst string) *packet.Parsed {
	return &packet.Parsed{
		IPVersion: 4,
		IPProto:   ipproto.UDP,
		Src:       netip.MustParseAddrPort(src),
		Dst:       netip.MustParseAddrPort(dst),
	}
}

func expectLine(t *testing.T, lines chan string, contains string) {
	t.Helper()
	select {
	case line := <-lines:
		if !strings.Contains(line, contains) {
			t.Errorf("got line %q, want it to contain %q", line, contains)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for a line containing %q", contains)
	}
}

func expectNoLine(t *testing.T, lines chan string) {
	t.Helper()
	select {
	case line := <-lines:
		t.Fatalf("unexpected line: %q", line)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestNewTCPFlowLogged(t *testing.T) {
	l, lines := newTestLogger(func(proto ipproto.Proto, src, dst netip.AddrPort) string {
		return "com.example.app"
	})
	l.observe(tcpSyn("10.0.0.1:5000", "10.0.0.2:443"))
	expectLine(t, lines, "owner=com.example.app")
}

func TestNewUDPFlowLogged(t *testing.T) {
	l, lines := newTestLogger(func(proto ipproto.Proto, src, dst netip.AddrPort) string {
		return "com.example.udp"
	})
	l.observe(udpPacket("10.0.0.1:1716", "10.0.0.2:1716"))
	expectLine(t, lines, "owner=com.example.udp")
}

func TestUnknownOwnerLogsPlaceholder(t *testing.T) {
	l, lines := newTestLogger(func(proto ipproto.Proto, src, dst netip.AddrPort) string {
		return ""
	})
	l.observe(tcpSyn("10.0.0.1:5000", "10.0.0.2:443"))
	expectLine(t, lines, "owner=?")
}

func TestRetransmitDeduped(t *testing.T) {
	l, lines := newTestLogger(func(proto ipproto.Proto, src, dst netip.AddrPort) string {
		return "pkg"
	})
	p := tcpSyn("10.0.0.1:5000", "10.0.0.2:443")
	l.observe(p)
	expectLine(t, lines, "owner=pkg")

	// SYN retransmit for the same 4-tuple must not log again.
	l.observe(p)
	expectNoLine(t, lines)
}

func TestNonSynTCPIgnored(t *testing.T) {
	l, lines := newTestLogger(func(proto ipproto.Proto, src, dst netip.AddrPort) string {
		return "pkg"
	})
	l.observe(tcpData("10.0.0.1:5000", "10.0.0.2:443"))
	expectNoLine(t, lines)
}

func TestChainedHookCalledAndVerdictReturned(t *testing.T) {
	l, lines := newTestLogger(func(proto ipproto.Proto, src, dst netip.AddrPort) string {
		return "pkg"
	})

	called := false
	w := &tstun.Wrapper{}
	w.PostFilterPacketOutboundToWireGuard = func(p *packet.Parsed, t *tstun.Wrapper) filter.Response {
		called = true
		return filter.Drop
	}
	l.Install(w)

	res := w.PostFilterPacketOutboundToWireGuard(tcpSyn("10.0.0.1:5000", "10.0.0.2:443"), w)
	if !called {
		t.Error("chained (previous) hook was not called")
	}
	if res != filter.Drop {
		t.Errorf("verdict = %v, want %v (the previous hook's verdict)", res, filter.Drop)
	}
	expectLine(t, lines, "owner=pkg")
}

func TestInstallPreservesNilPreviousHook(t *testing.T) {
	l, lines := newTestLogger(func(proto ipproto.Proto, src, dst netip.AddrPort) string {
		return "pkg"
	})

	w := &tstun.Wrapper{}
	l.Install(w)

	res := w.PostFilterPacketOutboundToWireGuard(tcpSyn("10.0.0.1:5000", "10.0.0.2:443"), w)
	if res != filter.Accept {
		t.Errorf("verdict = %v, want %v when no previous hook was set", res, filter.Accept)
	}
	expectLine(t, lines, "owner=pkg")
}

func TestRateLimitEngages(t *testing.T) {
	l, lines := newTestLogger(func(proto ipproto.Proto, src, dst netip.AddrPort) string {
		return "pkg"
	})

	// Burst is 20; fire more distinct new flows than that in one go.
	for i := range 25 {
		l.observe(tcpSyn(fmt.Sprintf("10.0.0.1:%d", 5000+i), "10.0.0.2:443"))
	}

	count := 0
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case <-lines:
			count++
		case <-timeout:
			break loop
		}
	}
	if count > 20 {
		t.Errorf("got %d logged flows, want <= 20 (the rate limiter burst)", count)
	}
	if count == 0 {
		t.Error("got 0 logged flows, want at least a few (the rate limiter burst)")
	}
}
