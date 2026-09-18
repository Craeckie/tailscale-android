// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package com.tailscale.ipn

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.os.PowerManager
import com.tailscale.ipn.util.TSLog

/**
 * Logs screen, doze and power-save-mode transitions into the local log ring, so an exported log can
 * tell real idle periods (screen off, deep doze) from wall-clock time.
 */
object PowerStateLogger {
  private const val TAG = "power"

  fun start(context: Context) {
    val appContext = context.applicationContext
    val powerManager = appContext.getSystemService(Context.POWER_SERVICE) as PowerManager

    val filter =
        IntentFilter().apply {
          addAction(Intent.ACTION_SCREEN_ON)
          addAction(Intent.ACTION_SCREEN_OFF)
          addAction(PowerManager.ACTION_DEVICE_IDLE_MODE_CHANGED)
          addAction(PowerManager.ACTION_POWER_SAVE_MODE_CHANGED)
          if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            addAction(PowerManager.ACTION_DEVICE_LIGHT_IDLE_MODE_CHANGED)
          }
        }
    val receiver =
        object : BroadcastReceiver() {
          override fun onReceive(receiverContext: Context?, intent: Intent?) {
            TSLog.d(TAG, format(powerManager, intent?.action ?: "unknown"))
          }
        }
    appContext.registerReceiver(receiver, filter)

    TSLog.d(TAG, format(powerManager, why = "start", pkg = appContext.packageName))
  }

  /** Pure formatting of the current power state; `pkg` is set only on the initial log line. */
  fun format(powerManager: PowerManager, why: String, pkg: String? = null): String {
    val screen = if (powerManager.isInteractive) "on" else "off"
    val doze = powerManager.isDeviceIdleMode
    val lightDoze =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
          powerManager.isDeviceLightIdleMode
        } else {
          false
        }
    val saver = powerManager.isPowerSaveMode
    val base = "screen=$screen doze=$doze lightDoze=$lightDoze saver=$saver why=$why"
    return if (pkg == null) {
      base
    } else {
      "$base ignoringBatteryOptimizations=${powerManager.isIgnoringBatteryOptimizations(pkg)}"
    }
  }
}
