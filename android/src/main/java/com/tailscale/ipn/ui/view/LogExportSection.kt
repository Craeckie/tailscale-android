// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Fork-only: the "Share logs" / "Save logs" rows on the bug report screen,
// kept out of BugReportView.kt so that file carries a one-line hook only.

package com.tailscale.ipn.ui.view

import android.content.ActivityNotFoundException
import android.content.Intent
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.core.content.FileProvider
import com.tailscale.ipn.BuildConfig
import com.tailscale.ipn.R
import com.tailscale.ipn.ui.util.Lists
import com.tailscale.ipn.ui.viewModel.BugReportViewModel
import com.tailscale.ipn.ui.viewModel.exportLogs
import com.tailscale.ipn.ui.viewModel.saveLogsTo
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

@Composable
fun LogExportSection(model: BugReportViewModel) {
  val context = LocalContext.current
  val noLogsText = stringResource(R.string.logs_none)

  val saveLogsLauncher =
      rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("text/plain")) { uri
        ->
        if (uri != null) {
          model.saveLogsTo(context, uri) { success ->
            if (!success) Toast.makeText(context, noLogsText, Toast.LENGTH_SHORT).show()
          }
        }
      }

  Lists.SectionDivider()

  Setting.Text(
      titleRes = R.string.share_logs,
      subtitle = stringResource(R.string.logs_subtitle),
      onClick = {
        model.exportLogs(context) { file ->
          if (file == null) {
            Toast.makeText(context, noLogsText, Toast.LENGTH_SHORT).show()
          } else {
            val uri =
                FileProvider.getUriForFile(context, "${BuildConfig.APPLICATION_ID}.logs", file)
            val intent =
                Intent(Intent.ACTION_SEND).apply {
                  type = "text/plain"
                  putExtra(Intent.EXTRA_STREAM, uri)
                  addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                }
            context.startActivity(Intent.createChooser(intent, null))
          }
        }
      })

  Setting.Text(
      titleRes = R.string.save_logs,
      onClick = {
        try {
          saveLogsLauncher.launch(defaultLogFileName())
        } catch (e: ActivityNotFoundException) {
          Toast.makeText(context, noLogsText, Toast.LENGTH_SHORT).show()
        }
      })
}

private fun defaultLogFileName(): String {
  val timestamp = SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US).format(Date())
  return "tailscale-logs-$timestamp.txt"
}
