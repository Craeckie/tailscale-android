// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package libtailscale

import "github.com/tailscale/tailscale-android/libtailscale/verboselog"

// verboseLogging is the Settings "Verbose logging" switch (fork-only).
var verboseLogging verboselog.Switch

// SetVerboseLogging sets whether "[v1] " log lines reach Logcat. Gomobile
// binds it as Libtailscale.setVerboseLogging(). Kotlin calls it with the
// persisted choice before Start, so the logger is created with it, and again
// on every toggle, which takes effect immediately.
func SetVerboseLogging(enabled bool) {
	verboseLogging.Set(enabled)
}
