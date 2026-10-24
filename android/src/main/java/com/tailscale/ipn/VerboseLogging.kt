// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package com.tailscale.ipn

import android.content.Context
import libtailscale.Libtailscale

/**
 * The Settings "Verbose logging" switch (fork-only): shows the backend's `[v1]` log lines in
 * Logcat. The local log ring that Share/Save logs exports has them either way, so the switch only
 * matters when reading Logcat live (adb logcat).
 */
object VerboseLogging {
  // Same file as App's unencrypted preferences.
  private const val PREFS = "unencrypted"
  private const val KEY = "isVerboseLoggingEnabled"

  fun isEnabled(context: Context): Boolean =
      context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getBoolean(KEY, false)

  /** Persists the choice and applies it to the running backend immediately. */
  fun setEnabled(context: Context, enabled: Boolean) {
    context
        .getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        .edit()
        .putBoolean(KEY, enabled)
        .apply()
    Libtailscale.setVerboseLogging(enabled)
  }

  /**
   * Hands the persisted choice to Go; call before Libtailscale.start so the logger starts with it.
   */
  fun applyPersisted(context: Context) {
    Libtailscale.setVerboseLogging(isEnabled(context))
  }
}
