// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Package wedgewatch is a fork-only watchdog for netstack's reply path to
// the host. It periodically sends a real DNS query to 100.100.100.100 from
// an ordinary socket, so the query enters through the TUN like any app's,
// and the answer has to come back through netstack → injection → TUN. When
// that path stops delivering while the rest of the engine keeps running, the
// watchdog logs a "wedge:" line with a goroutine dump and the netstack/tstun
// counters, once per episode, so the stuck goroutine is visible in the local
// log ring afterwards.
package wedgewatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime/pprof"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"tailscale.com/types/logger"
	"tailscale.com/util/clientmetric"
)

// ErrInactive is returned by a Config.Probe that decided probing makes no
// sense right now (backend not running, Tailscale DNS off). Like any error
// that is not a timeout, it is neither a success nor a failure and resets
// the failure count.
var ErrInactive = errors.New("wedgewatch: inactive")

// errStuck is reported for a probe that did not return within Timeout plus
// a grace period, or that is still running from an earlier round.
var errStuck = errors.New("probe did not return")

// Config configures a Watcher. Probe and Logf are required.
type Config struct {
	// Interval between probes.
	Interval time.Duration
	// Timeout for one probe; Probe gets a context with this deadline.
	Timeout time.Duration
	// Threshold is the number of consecutive failures that make a wedge.
	Threshold int
	// MaxDumps caps the number of dumps per process lifetime.
	MaxDumps int
	// Probe performs one round trip. It returns nil on success, ErrInactive
	// if probing makes no sense right now, any other error on failure.
	Probe func(context.Context) error
	// Logf receives the "wedge:" lines.
	Logf logger.Logf
	// Dump writes the diagnostic dump; defaults to WriteDump.
	Dump func(io.Writer)
}

// Watcher runs Config.Probe in a loop and reports wedges.
type Watcher struct {
	cfg      Config
	inFlight atomic.Bool

	failures int
	dumped   bool // dumped for the current episode
	dumps    int
}

// New returns a Watcher for cfg, filling in defaults.
func New(cfg Config) *Watcher {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = 2
	}
	if cfg.MaxDumps <= 0 {
		cfg.MaxDumps = 3
	}
	if cfg.Dump == nil {
		cfg.Dump = WriteDump
	}
	return &Watcher{cfg: cfg}
}

// Run probes every Interval until ctx is done.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.Round(ctx)
		}
	}
}

// Round runs one probe (unless the previous one is still stuck) and updates
// the wedge state. It is exported for tests.
func (w *Watcher) Round(ctx context.Context) {
	w.record(w.probeOnce(ctx))
}

// probeOnce runs Probe in its own goroutine so that a probe blocked on a
// wedged lock (LocalBackend's mutex, a full queue) cannot block the watcher
// itself: the round then counts as a failure, and later rounds keep counting
// failures without starting another probe until the stuck one returns.
func (w *Watcher) probeOnce(ctx context.Context) error {
	if !w.inFlight.CompareAndSwap(false, true) {
		return errStuck
	}
	res := make(chan error, 1) // buffered: a stuck probe's late result is dropped
	go func() {
		pctx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
		err := w.cfg.Probe(pctx)
		cancel()
		// Clear inFlight before handing over the result, so the next round
		// never sees a finished probe as still running.
		w.inFlight.Store(false)
		res <- err
	}()
	grace := time.NewTimer(w.cfg.Timeout + time.Second)
	defer grace.Stop()
	select {
	case err := <-res:
		return err
	case <-grace.C:
		return errStuck
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Watcher) record(err error) {
	switch {
	case errors.Is(err, context.Canceled):
		return
	case err == nil:
		if w.failures >= w.cfg.Threshold {
			w.cfg.Logf("wedge: recovered after %d failed quad-100 probes", w.failures)
		}
		w.reset()
		return
	case !isTimeout(err):
		// ErrInactive, or a fast error (EPERM while Doze blocks the app,
		// "network unreachable" while the tunnel is rebuilt, a bad rcode):
		// the probe got no verdict on the reply path, which shows up as a
		// timeout when it is stuck. End any episode without calling it a
		// recovery.
		if w.failures >= w.cfg.Threshold {
			w.cfg.Logf("wedge: probing interrupted after %d failed quad-100 probes: %v", w.failures, err)
		}
		w.reset()
		return
	}
	w.failures++
	if w.failures < w.cfg.Threshold || w.dumped {
		return
	}
	w.dumped = true
	if w.dumps >= w.cfg.MaxDumps {
		w.cfg.Logf("wedge: %d consecutive quad-100 probes failed (last: %v); dump limit %d reached", w.failures, err, w.cfg.MaxDumps)
		return
	}
	w.dumps++
	var buf bytes.Buffer
	w.cfg.Dump(&buf)
	w.cfg.Logf("wedge: %d consecutive quad-100 probes failed (last: %v); dump %d/%d follows\n%s", w.failures, err, w.dumps, w.cfg.MaxDumps, buf.String())
}

func (w *Watcher) reset() {
	w.failures = 0
	w.dumped = false
}

// isTimeout reports whether err means the probe got no answer in time.
func isTimeout(err error) bool {
	if errors.Is(err, errStuck) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// WriteDump writes the netstack, tstun and DNS clientmetrics and a full
// goroutine dump (debug=2, with wait reasons and durations) to out.
func WriteDump(out io.Writer) {
	var names []string
	vals := map[string]int64{}
	for _, m := range clientmetric.Metrics() {
		n := m.Name()
		if strings.HasPrefix(n, "netstack_") || strings.HasPrefix(n, "tstun_") || strings.HasPrefix(n, "dns_") {
			names = append(names, n)
			vals[n] = m.Value()
		}
	}
	sort.Strings(names)
	fmt.Fprintf(out, "wedge metrics:")
	for _, n := range names {
		fmt.Fprintf(out, " %s=%d", n, vals[n])
	}
	fmt.Fprintf(out, "\n")
	if p := pprof.Lookup("goroutine"); p != nil {
		p.WriteTo(out, 2)
	}
}
