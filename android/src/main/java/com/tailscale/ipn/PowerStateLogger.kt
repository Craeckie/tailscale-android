// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package com.tailscale.ipn

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.BatteryManager
import android.os.Build
import android.os.PowerManager
import com.tailscale.ipn.util.TSLog
import libtailscale.Libtailscale

/**
 * Logs screen, doze, power-save-mode and charger transitions into the local log ring, so an
 * exported log can tell real idle periods (screen off, deep doze) from wall-clock time, and battery
 * drain from time on the charger. A separate `battery:` line records each change of the battery
 * level or plug state, so drain per state can be computed from the log alone.
 */
object PowerStateLogger {
  private const val TAG = "power"
  private const val BATTERY_TAG = "battery"

  // BatteryManager.BATTERY_PLUGGED_DOCK is API 33+; compared by value so older levels compile.
  private const val PLUGGED_DOCK = 8

  /** Battery level in percent (-1 if unknown) and plug state name. */
  data class BatteryState(val level: Int, val plugged: String)

  fun start(context: Context) {
    val appContext = context.applicationContext
    val powerManager = appContext.getSystemService(Context.POWER_SERVICE) as PowerManager

    val filter =
        IntentFilter().apply {
          addAction(Intent.ACTION_SCREEN_ON)
          addAction(Intent.ACTION_SCREEN_OFF)
          addAction(Intent.ACTION_POWER_CONNECTED)
          addAction(Intent.ACTION_POWER_DISCONNECTED)
          addAction(PowerManager.ACTION_DEVICE_IDLE_MODE_CHANGED)
          addAction(PowerManager.ACTION_POWER_SAVE_MODE_CHANGED)
          if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            addAction(PowerManager.ACTION_DEVICE_LIGHT_IDLE_MODE_CHANGED)
          }
        }
    val receiver =
        object : BroadcastReceiver() {
          override fun onReceive(receiverContext: Context?, intent: Intent?) {
            val action = intent?.action ?: "unknown"
            val battery = withAction(action, readBattery(appContext))
            TSLog.d(TAG, format(powerManager, action, battery))
            // Closes the current metricslog delta row here, so it never straddles this
            // transition: the deltas attribute cleanly to the state before vs. after it.
            Libtailscale.metricsTick()
          }
        }
    appContext.registerReceiver(receiver, filter)

    // ACTION_BATTERY_CHANGED is sticky and also fires on voltage/temperature changes; the dedupe
    // keeps one line per level or plug change. No metricsTick() here: plug changes already tick
    // through the receiver above, and a tick per percent would flood the metrics rows.
    val dedupe = BatteryDedupe()
    val batteryReceiver =
        object : BroadcastReceiver() {
          override fun onReceive(receiverContext: Context?, intent: Intent?) {
            val state = intent?.let { batteryState(it) } ?: return
            if (dedupe.shouldLog(state)) {
              TSLog.d(BATTERY_TAG, formatBattery(state))
            }
          }
        }
    appContext.registerReceiver(batteryReceiver, IntentFilter(Intent.ACTION_BATTERY_CHANGED))

    TSLog.d(
        TAG,
        format(
            powerManager,
            why = "start",
            battery = readBattery(appContext),
            pkg = appContext.packageName,
        ),
    )
  }

  /** One-off read of the sticky ACTION_BATTERY_CHANGED intent; null if none is available. */
  private fun readBattery(context: Context): BatteryState? =
      context.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))?.let {
        batteryState(it)
      }

  private fun batteryState(intent: Intent): BatteryState =
      batteryState(
          level = intent.getIntExtra(BatteryManager.EXTRA_LEVEL, -1),
          scale = intent.getIntExtra(BatteryManager.EXTRA_SCALE, -1),
          pluggedExtra = intent.getIntExtra(BatteryManager.EXTRA_PLUGGED, 0),
      )

  /** Pure mapping of the ACTION_BATTERY_CHANGED extras. */
  fun batteryState(level: Int, scale: Int, pluggedExtra: Int): BatteryState {
    val percent = if (level < 0 || scale <= 0) -1 else level * 100 / scale
    return BatteryState(percent, pluggedName(pluggedExtra))
  }

  fun pluggedName(pluggedExtra: Int): String =
      when (pluggedExtra) {
        BatteryManager.BATTERY_PLUGGED_AC -> "ac"
        BatteryManager.BATTERY_PLUGGED_USB -> "usb"
        BatteryManager.BATTERY_PLUGGED_WIRELESS -> "wireless"
        PLUGGED_DOCK -> "dock"
        else -> "none"
      }

  /**
   * The sticky battery intent can lag behind ACTION_POWER_CONNECTED/DISCONNECTED, so the action
   * wins: disconnect means `none`; connect with a sticky `none` means `yes` (type not known yet,
   * the next `battery:` line carries it).
   */
  fun withAction(action: String, battery: BatteryState?): BatteryState? {
    if (battery == null) return null
    return when {
      action == Intent.ACTION_POWER_DISCONNECTED -> battery.copy(plugged = "none")
      action == Intent.ACTION_POWER_CONNECTED && battery.plugged == "none" ->
          battery.copy(plugged = "yes")
      else -> battery
    }
  }

  /**
   * Pure formatting of the current power state. `battery` fields go after `why=` so parsers that
   * cut at ` saver` keep their state keys; `pkg` is set only on the initial log line.
   */
  fun format(
      powerManager: PowerManager,
      why: String,
      battery: BatteryState? = null,
      pkg: String? = null,
  ): String {
    val screen = if (powerManager.isInteractive) "on" else "off"
    val doze = powerManager.isDeviceIdleMode
    val lightDoze =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
          powerManager.isDeviceLightIdleMode
        } else {
          false
        }
    val saver = powerManager.isPowerSaveMode
    var line = "screen=$screen doze=$doze lightDoze=$lightDoze saver=$saver why=$why"
    if (battery != null) {
      line += " plugged=${battery.plugged} level=${battery.level}"
    }
    if (pkg != null) {
      line += " ignoringBatteryOptimizations=${powerManager.isIgnoringBatteryOptimizations(pkg)}"
    }
    return line
  }

  fun formatBattery(state: BatteryState): String = "level=${state.level} plugged=${state.plugged}"
}

/** Passes a battery state through only when its (level, plugged) pair differs from the last one. */
class BatteryDedupe {
  private var last: PowerStateLogger.BatteryState? = null

  @Synchronized
  fun shouldLog(state: PowerStateLogger.BatteryState): Boolean {
    if (state == last) return false
    last = state
    return true
  }
}
