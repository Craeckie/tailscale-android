// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package com.tailcale.ipn

import android.os.PowerManager
import com.tailscale.ipn.PowerStateLogger
import org.junit.Assert.assertEquals
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

    val line = PowerStateLogger.format(powerManager, why = "ACTION_DEVICE_IDLE_MODE_CHANGED")

    assertEquals(
        "screen=off doze=true lightDoze=false saver=false why=ACTION_DEVICE_IDLE_MODE_CHANGED",
        line)
  }

  @Test
  fun `format reports screen on and no doze`() {
    val powerManager: PowerManager = mock()
    whenever(powerManager.isInteractive).thenReturn(true)
    whenever(powerManager.isDeviceIdleMode).thenReturn(false)
    whenever(powerManager.isPowerSaveMode).thenReturn(false)

    val line = PowerStateLogger.format(powerManager, why = "android.intent.action.SCREEN_ON")

    assertEquals(
        "screen=on doze=false lightDoze=false saver=false why=android.intent.action.SCREEN_ON",
        line)
  }

  @Test
  fun `format includes ignoringBatteryOptimizations only when pkg is given`() {
    val powerManager: PowerManager = mock()
    whenever(powerManager.isInteractive).thenReturn(true)
    whenever(powerManager.isDeviceIdleMode).thenReturn(false)
    whenever(powerManager.isPowerSaveMode).thenReturn(false)
    whenever(powerManager.isIgnoringBatteryOptimizations("com.tailscale.ipn")).thenReturn(true)

    val line = PowerStateLogger.format(powerManager, why = "start", pkg = "com.tailscale.ipn")

    assertEquals(
        "screen=on doze=false lightDoze=false saver=false why=start ignoringBatteryOptimizations=true",
        line)
  }
}
