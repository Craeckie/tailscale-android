// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package com.tailscale.ipn

import android.content.Intent
import android.os.PowerManager
import com.tailscale.ipn.PowerStateLogger.BatteryState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.mockito.kotlin.mock
import org.mockito.kotlin.whenever

class PowerStateLoggerTest {
  @Test
  fun `format reports screen off and doze`() {
    val powerManager: PowerManager = mock()
    whenever(powerManager.isInteractive).thenReturn(false)
    whenever(powerManager.isDeviceIdleMode).thenReturn(true)
    whenever(powerManager.isPowerSaveMode).thenReturn(false)

    val line =
        PowerStateLogger.format(
            powerManager,
            why = "ACTION_DEVICE_IDLE_MODE_CHANGED",
            battery = BatteryState(54, "none"),
        )

    assertEquals(
        "screen=off doze=true lightDoze=false saver=false why=ACTION_DEVICE_IDLE_MODE_CHANGED plugged=none level=54",
        line,
    )
  }

  @Test
  fun `format reports screen on and no doze`() {
    val powerManager: PowerManager = mock()
    whenever(powerManager.isInteractive).thenReturn(true)
    whenever(powerManager.isDeviceIdleMode).thenReturn(false)
    whenever(powerManager.isPowerSaveMode).thenReturn(false)

    val line =
        PowerStateLogger.format(
            powerManager,
            why = "android.intent.action.SCREEN_ON",
            battery = BatteryState(100, "ac"),
        )

    assertEquals(
        "screen=on doze=false lightDoze=false saver=false why=android.intent.action.SCREEN_ON plugged=ac level=100",
        line,
    )
  }

  @Test
  fun `format includes ignoringBatteryOptimizations only when pkg is given`() {
    val powerManager: PowerManager = mock()
    whenever(powerManager.isInteractive).thenReturn(true)
    whenever(powerManager.isDeviceIdleMode).thenReturn(false)
    whenever(powerManager.isPowerSaveMode).thenReturn(false)
    whenever(powerManager.isIgnoringBatteryOptimizations("com.tailscale.ipn")).thenReturn(true)

    val line =
        PowerStateLogger.format(
            powerManager,
            why = "start",
            battery = BatteryState(80, "usb"),
            pkg = "com.tailscale.ipn",
        )

    assertEquals(
        "screen=on doze=false lightDoze=false saver=false why=start plugged=usb level=80 ignoringBatteryOptimizations=true",
        line,
    )
  }

  @Test
  fun `format omits battery fields when the state is unknown`() {
    val powerManager: PowerManager = mock()
    whenever(powerManager.isInteractive).thenReturn(false)

    val line = PowerStateLogger.format(powerManager, why = "x")

    assertEquals("screen=off doze=false lightDoze=false saver=false why=x", line)
  }

  @Test
  fun `pluggedName maps every plug type, dock by value, unknown to none`() {
    assertEquals("ac", PowerStateLogger.pluggedName(1))
    assertEquals("usb", PowerStateLogger.pluggedName(2))
    assertEquals("wireless", PowerStateLogger.pluggedName(4))
    assertEquals("dock", PowerStateLogger.pluggedName(8))
    assertEquals("none", PowerStateLogger.pluggedName(0))
    assertEquals("none", PowerStateLogger.pluggedName(99))
  }

  @Test
  fun `batteryState scales the level and handles missing extras`() {
    assertEquals(BatteryState(42, "usb"), PowerStateLogger.batteryState(42, 100, 2))
    assertEquals(BatteryState(50, "none"), PowerStateLogger.batteryState(128, 256, 0))
    assertEquals(BatteryState(-1, "none"), PowerStateLogger.batteryState(-1, -1, 0))
  }

  @Test
  fun `the plug action wins over a stale sticky battery intent`() {
    val stalePlugged = BatteryState(60, "ac")
    val staleUnplugged = BatteryState(60, "none")

    assertEquals(
        BatteryState(60, "none"),
        PowerStateLogger.withAction(Intent.ACTION_POWER_DISCONNECTED, stalePlugged),
    )
    assertEquals(
        BatteryState(60, "yes"),
        PowerStateLogger.withAction(Intent.ACTION_POWER_CONNECTED, staleUnplugged),
    )
    // A sticky value that already knows the type is kept on connect.
    assertEquals(
        stalePlugged,
        PowerStateLogger.withAction(Intent.ACTION_POWER_CONNECTED, stalePlugged),
    )
    // Other actions pass the sticky value through unchanged.
    assertEquals(
        stalePlugged,
        PowerStateLogger.withAction(Intent.ACTION_SCREEN_OFF, stalePlugged),
    )
    assertNull(PowerStateLogger.withAction(Intent.ACTION_POWER_CONNECTED, null))
  }

  @Test
  fun `BatteryDedupe logs only level or plug changes`() {
    val dedupe = BatteryDedupe()
    assertTrue(dedupe.shouldLog(BatteryState(80, "none")))
    assertFalse(dedupe.shouldLog(BatteryState(80, "none")))
    assertTrue(dedupe.shouldLog(BatteryState(79, "none")))
    assertTrue(dedupe.shouldLog(BatteryState(79, "usb")))
    assertFalse(dedupe.shouldLog(BatteryState(79, "usb")))
  }

  @Test
  fun `formatBattery puts level before plugged`() {
    assertEquals(
        "level=7 plugged=wireless",
        PowerStateLogger.formatBattery(BatteryState(7, "wireless")),
    )
  }
}
