// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Package verboselog holds the fork's "Verbose logging" switch.
//
// logtail writes a line to its Stderr (Logcat on Android) only when the
// line's level is at most the logger's stderr level. Upstream leaves that
// level at 0, so every "[v1] " line in tailscale.com is missing from Logcat.
// The switch raises it to 1. The local log ring and the upload buffer get
// those lines either way; this only changes what Logcat shows.
package verboselog

import "sync/atomic"

// Logger is the part of *logtail.Logger the switch needs.
type Logger interface {
	SetVerbosityLevel(level int)
}

// Switch remembers the user's choice and applies it to the logger once one
// is attached. The zero value is off with no logger. Safe for concurrent use.
type Switch struct {
	enabled atomic.Bool
	logger  atomic.Pointer[Logger]
}

// Set records the choice and applies it to the attached logger, if any.
func (s *Switch) Set(enabled bool) {
	s.enabled.Store(enabled)
	if p := s.logger.Load(); p != nil {
		(*p).SetVerbosityLevel(s.Level())
	}
}

// Attach makes lg the logger that Set updates and applies the current
// choice to it.
func (s *Switch) Attach(lg Logger) {
	s.logger.Store(&lg)
	lg.SetVerbosityLevel(s.Level())
}

// Level is the logtail stderr level for the current choice: 1 when
// enabled, 0 otherwise.
func (s *Switch) Level() int {
	if s.enabled.Load() {
		return 1
	}
	return 0
}
