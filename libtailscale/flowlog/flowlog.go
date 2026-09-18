// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Package flowlog logs, rate-limited, the Android app that owns each new
// TCP/UDP flow entering the tunnel, so battery and traffic investigations
// can attribute tunnel traffic to a package without adb.
package flowlog

import (
	"net/netip"
	"sync"
	"time"

	"tailscale.com/net/flowtrack"
	"tailscale.com/net/packet"
	"tailscale.com/net/tstun"
	"tailscale.com/tstime/rate"
	"tailscale.com/types/ipproto"
	"tailscale.com/types/logger"
	"tailscale.com/util/clientmetric"
	"tailscale.com/util/lru"
	"tailscale.com/wgengine/filter"
)

const (
	// dedupeTTL bounds how often a still-open flow (a long UDP stream, or a
	// TCP connection whose SYN keeps retransmitting) is re-logged.
	dedupeTTL = 5 * time.Minute
	dedupeCap = 1024

	ownerQueueCap = 128
)

var (
	metricNewFlows = clientmetric.NewCounter("fork_flowlog_new_flows")
	metricDropped  = clientmetric.NewCounter("fork_flowlog_dropped")
)

// OwnerFunc resolves the app package(s) behind a flow's local endpoint. It
// must return promptly ("" if unknown) since it runs off the packet path but
// still needs to keep up with new flows.
type OwnerFunc func(proto ipproto.Proto, src, dst netip.AddrPort) string

// Logger observes newly-opened outbound flows and logs, via logf, the app
// that owns each one.
type Logger struct {
	logf  logger.Logf
	owner OwnerFunc

	limiter *rate.Limiter

	mu   sync.Mutex
	seen lru.Cache[flowtrack.Tuple, time.Time]

	lookups chan lookupReq
}

type lookupReq struct {
	proto    ipproto.Proto
	src, dst netip.AddrPort
}

// New creates a Logger that calls owner to resolve the app behind each new
// flow and writes one line per flow via logf.
func New(logf logger.Logf, owner OwnerFunc) *Logger {
	l := &Logger{
		logf:    logf,
		owner:   owner,
		limiter: rate.NewLimiter(rate.Every(200*time.Millisecond), 20),
		lookups: make(chan lookupReq, ownerQueueCap),
	}
	l.seen.MaxEntries = dedupeCap
	go l.drainLookups()
	return l
}

// Install chains l onto w's PostFilterPacketOutboundToWireGuard hook,
// preserving whatever hook w already had installed (wgengine installs its
// own flow tracker there), so both observe every accepted outbound packet.
func (l *Logger) Install(w *tstun.Wrapper) {
	prev := w.PostFilterPacketOutboundToWireGuard
	w.PostFilterPacketOutboundToWireGuard = func(p *packet.Parsed, t *tstun.Wrapper) filter.Response {
		l.observe(p)
		if prev != nil {
			return prev(p, t)
		}
		return filter.Accept
	}
}

func (l *Logger) observe(p *packet.Parsed) {
	if p.IPVersion == 0 {
		return
	}
	switch p.IPProto {
	case ipproto.TCP:
		if !p.IsTCPSyn() {
			return
		}
	case ipproto.UDP:
		// No per-packet signal for "new" UDP flow; the dedupe cache below
		// keeps this to one log line per flow per dedupeTTL.
	default:
		return
	}

	tuple := flowtrack.MakeTuple(p.IPProto, p.Src, p.Dst)

	l.mu.Lock()
	if last, ok := l.seen.GetOk(tuple); ok && time.Since(last) < dedupeTTL {
		l.mu.Unlock()
		return
	}
	l.seen.Set(tuple, time.Now())
	l.mu.Unlock()

	if !l.limiter.Allow() {
		metricDropped.Add(1)
		return
	}
	metricNewFlows.Add(1)

	select {
	case l.lookups <- lookupReq{p.IPProto, p.Src, p.Dst}:
	default:
		// The owner-lookup goroutine is behind; drop rather than block the
		// packet path.
		metricDropped.Add(1)
	}
}

func (l *Logger) drainLookups() {
	for req := range l.lookups {
		owner := "?"
		if o := l.owner(req.proto, req.src, req.dst); o != "" {
			owner = o
		}
		proto := "UDP"
		if req.proto == ipproto.TCP {
			proto = "TCP"
		}
		l.logf("flow: %s %s > %s owner=%s", proto, req.src, req.dst, owner)
	}
}
