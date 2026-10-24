// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package verboselog

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"tailscale.com/logtail"
)

// syncBuffer is a bytes.Buffer safe for logtail's concurrent writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) takeString() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.buf.String()
	b.buf.Reset()
	return s
}

// newLogger returns a real logtail.Logger that never uploads and writes its
// stderr output to the returned buffer, as libtailscale's setupLogs does with
// Logcat.
func newLogger(t *testing.T) (*logtail.Logger, *syncBuffer) {
	t.Helper()
	stderr := new(syncBuffer)
	lg := logtail.NewLogger(logtail.Config{
		Collection: logtail.CollectionNode,
		Stderr:     stderr,
		Disabled:   true,
	}, t.Logf)
	t.Cleanup(func() { lg.Shutdown(t.Context()) })
	return lg, stderr
}

func TestVerboseLinesReachStderrOnlyWhenEnabled(t *testing.T) {
	lg, stderr := newLogger(t)
	var s Switch
	s.Attach(lg)

	lg.Logf("[v1] hidden while off")
	lg.Logf("normal line")
	got := stderr.takeString()
	if strings.Contains(got, "hidden while off") {
		t.Errorf("off: [v1] line reached stderr: %q", got)
	}
	if !strings.Contains(got, "normal line") {
		t.Errorf("off: normal line missing from stderr: %q", got)
	}

	s.Set(true)
	lg.Logf("[v1] shown while on")
	lg.Logf("[v2] still hidden")
	got = stderr.takeString()
	if !strings.Contains(got, "shown while on") {
		t.Errorf("on: [v1] line missing from stderr: %q", got)
	}
	if strings.Contains(got, "[v1]") {
		t.Errorf("on: level tag not stripped: %q", got)
	}
	if strings.Contains(got, "still hidden") {
		t.Errorf("on: [v2] line reached stderr: %q", got)
	}

	s.Set(false)
	lg.Logf("[v1] hidden again")
	if got := stderr.takeString(); strings.Contains(got, "hidden again") {
		t.Errorf("off again: [v1] line reached stderr: %q", got)
	}
}

// The persisted choice is set before the logger exists (Kotlin calls
// SetVerboseLogging before Libtailscale.start); Attach must apply it.
func TestSetBeforeAttach(t *testing.T) {
	var s Switch
	s.Set(true)
	lg, stderr := newLogger(t)
	s.Attach(lg)
	lg.Logf("[v1] early choice applied")
	if got := stderr.takeString(); !strings.Contains(got, "early choice applied") {
		t.Errorf("[v1] line missing from stderr: %q", got)
	}
}

type fakeLogger struct{ levels []int }

func (f *fakeLogger) SetVerbosityLevel(level int) { f.levels = append(f.levels, level) }

func TestSetWithoutLogger(t *testing.T) {
	var s Switch
	s.Set(true) // must not panic
	if s.Level() != 1 {
		t.Fatalf("Level() = %d, want 1", s.Level())
	}
	f := new(fakeLogger)
	s.Attach(f)
	s.Set(false)
	if want := []int{1, 0}; len(f.levels) != 2 || f.levels[0] != want[0] || f.levels[1] != want[1] {
		t.Errorf("levels = %v, want %v", f.levels, want)
	}
}
