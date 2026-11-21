// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package wedgewatch

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type lines struct {
	mu sync.Mutex
	l  []string
}

func (l *lines) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.l = append(l.l, fmt.Sprintf(format, args...))
}

func (l *lines) count(prefix string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.l {
		if strings.HasPrefix(s, prefix) {
			n++
		}
	}
	return n
}

func newTestWatcher(probe func(context.Context) error, lg *lines, dumps *int) *Watcher {
	return New(Config{
		Timeout:   20 * time.Millisecond,
		Threshold: 2,
		MaxDumps:  2,
		Probe:     probe,
		Logf:      lg.logf,
		Dump:      func(w io.Writer) { *dumps++; io.WriteString(w, "DUMP") },
	})
}

func TestHangingProbeDumpsOnce(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	var lg lines
	dumps := 0
	w := newTestWatcher(func(ctx context.Context) error {
		<-release // ignores ctx, like a probe stuck on a lock
		return nil
	}, &lg, &dumps)

	for range 5 {
		w.Round(context.Background())
	}
	if dumps != 1 {
		t.Fatalf("dumps = %d, want 1", dumps)
	}
	if got := lg.count("wedge: 2 consecutive"); got != 1 {
		t.Fatalf("wedge lines = %d, want 1: %q", got, lg.l)
	}
	if !strings.Contains(lg.l[0], "probe did not return") || !strings.HasSuffix(lg.l[0], "DUMP") {
		t.Fatalf("unexpected wedge line %q", lg.l[0])
	}
}

func TestRecoveryStartsNewEpisodeAndCapsDumps(t *testing.T) {
	var lg lines
	dumps := 0
	fail := true
	w := newTestWatcher(func(ctx context.Context) error {
		if fail {
			return os.ErrDeadlineExceeded
		}
		return nil
	}, &lg, &dumps)

	episode := func() {
		fail = true
		w.Round(context.Background())
		w.Round(context.Background())
		w.Round(context.Background())
		fail = false
		w.Round(context.Background())
	}
	episode()
	episode()
	episode()
	if dumps != 2 {
		t.Fatalf("dumps = %d, want 2 (MaxDumps)", dumps)
	}
	if got := lg.count("wedge: recovered after 3 failed"); got != 3 {
		t.Fatalf("recovered lines = %d, want 3: %q", got, lg.l)
	}
	if got := lg.count("wedge: 2 consecutive quad-100 probes failed (last: i/o timeout); dump limit"); got != 1 {
		t.Fatalf("limit lines = %d, want 1: %q", got, lg.l)
	}
}

func TestInactiveAndSingleFailureAreQuiet(t *testing.T) {
	var lg lines
	dumps := 0
	results := []error{os.ErrDeadlineExceeded, ErrInactive, os.ErrDeadlineExceeded, nil, os.ErrDeadlineExceeded}
	i := 0
	w := newTestWatcher(func(ctx context.Context) error { err := results[i]; i++; return err }, &lg, &dumps)
	for range results {
		w.Round(context.Background())
	}
	if dumps != 0 || len(lg.l) != 0 {
		t.Fatalf("dumps=%d lines=%q, want none", dumps, lg.l)
	}
}

func TestFastErrorsAreNotWedges(t *testing.T) {
	var lg lines
	dumps := 0
	perm := &net.OpError{Op: "write", Net: "udp", Err: os.ErrPermission}
	results := []error{os.ErrDeadlineExceeded, os.ErrDeadlineExceeded, perm, perm, perm, errors.New("rcode 2")}
	i := 0
	w := newTestWatcher(func(ctx context.Context) error { err := results[i]; i++; return err }, &lg, &dumps)
	for range results {
		w.Round(context.Background())
	}
	if dumps != 1 {
		t.Fatalf("dumps = %d, want 1 (only the timeout streak)", dumps)
	}
	if got := lg.count("wedge: probing interrupted after 2 failed"); got != 1 {
		t.Fatalf("interrupted lines = %d, want 1: %q", got, lg.l)
	}
	if got := lg.count("wedge: recovered"); got != 0 {
		t.Fatalf("a fast error was logged as recovery: %q", lg.l)
	}
}

func TestWriteDump(t *testing.T) {
	var b strings.Builder
	WriteDump(&b)
	s := b.String()
	if !strings.HasPrefix(s, "wedge metrics:") || !strings.Contains(s, "goroutine ") || !strings.Contains(s, "TestWriteDump") {
		t.Fatalf("dump lacks metrics header or goroutine stacks:\n%.500s", s)
	}
}

// fakeResolver answers A queries for ProbeName with ip, or swallows them.
func fakeResolver(t *testing.T, answer bool, ip [4]byte) netip.AddrPort {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if !answer {
				continue
			}
			q := buf[:n]
			qend, err := skipName(q, 12)
			if err != nil || qend+4 > n {
				continue
			}
			id := binary.BigEndian.Uint16(q)
			// A stray reply with the wrong ID first, which must be skipped.
			for _, rid := range []uint16{id + 1, id} {
				r := append([]byte(nil), q[:qend+4]...)
				binary.BigEndian.PutUint16(r[0:], rid)
				r[2] |= 0x80                         // QR
				binary.BigEndian.PutUint16(r[6:], 1) // ANCOUNT
				r = append(r, 0xC0, 12, 0, typeA, 0, classIN, 0, 0, 0, 60, 0, 4)
				r = append(r, ip[:]...)
				pc.WriteTo(r, addr)
			}
		}
	}()
	return netip.MustParseAddrPort(pc.LocalAddr().String())
}

func TestQueryA(t *testing.T) {
	want := netip.MustParseAddr("100.100.100.100")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := QueryA(ctx, fakeResolver(t, true, want.As4()), want); err != nil {
		t.Errorf("answering resolver: %v", err)
	}
	if err := QueryA(ctx, fakeResolver(t, true, [4]byte{1, 2, 3, 4}), want); err == nil {
		t.Errorf("wrong address: got nil error")
	}
	sctx, scancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer scancel()
	if err := QueryA(sctx, fakeResolver(t, false, want.As4()), want); err == nil {
		t.Errorf("silent resolver: got nil error")
	}
}
