// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Fork-only: log export actions for the bug report screen, kept out of
// BugReportViewModel.kt so that file stays identical to upstream.

package com.tailscale.ipn.ui.viewModel

import android.content.Context
import android.net.Uri
import androidx.lifecycle.viewModelScope
import com.tailscale.ipn.util.LogExport
import java.io.File
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

fun BugReportViewModel.exportLogs(context: Context, onResult: (File?) -> Unit) {
  val appContext = context.applicationContext
  viewModelScope.launch {
    val file = withContext(Dispatchers.IO) { LogExport.export(appContext) }
    onResult(file)
  }
}

fun BugReportViewModel.saveLogsTo(context: Context, uri: Uri, onResult: (Boolean) -> Unit) {
  val appContext = context.applicationContext
  viewModelScope.launch {
    val success =
        withContext(Dispatchers.IO) {
          val files = LogExport.bufferFiles(appContext.filesDir)
          if (files.isEmpty()) {
            false
          } else {
            runCatching {
                  appContext.contentResolver.openOutputStream(uri)?.use {
                    LogExport.writeTo(files, it)
                  } ?: throw IOException("openOutputStream returned null for $uri")
                }
                .isSuccess
          }
        }
    onResult(success)
  }
}
