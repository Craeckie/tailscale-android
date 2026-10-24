// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Fork-only: the "Verbose logging" row in Settings, kept out of SettingsView.kt so that file
// carries a one-line hook only.

package com.tailscale.ipn.ui.view

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import com.tailscale.ipn.R
import com.tailscale.ipn.VerboseLogging
import com.tailscale.ipn.ui.util.Lists

@Composable
fun VerboseLoggingSetting() {
  val context = LocalContext.current
  var isOn by remember { mutableStateOf(VerboseLogging.isEnabled(context)) }

  Lists.ItemDivider()
  Setting.Switch(
      R.string.verbose_logging,
      subtitle = stringResource(R.string.verbose_logging_subtitle),
      isOn = isOn,
      onToggle = { enabled ->
        isOn = enabled
        VerboseLogging.setEnabled(context, enabled)
      },
  )
}
