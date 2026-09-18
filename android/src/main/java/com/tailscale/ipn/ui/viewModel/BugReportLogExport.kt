// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Fork-only: log export actions for the bug report screen, kept out of
// BugReportViewModel.kt so that file stays identical to upstream.

package com.tailscale.ipn.ui.viewModel

import android.content.Context
import android.net.Uri
import androidx.lifecycle.viewModelScope
import com.tailscale.ipn.ui.localapi.Request
import com.tailscale.ipn.util.LogExport
import java.io.File
import java.io.IOException
import kotlin.reflect.typeOf
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext

/**
 * Fetches a /localapi/v0/metrics snapshot to prepend to an export, giving the absolute counters at
 * export time to cross-check metricslog's deltas against. Export must still work if the backend
 * can't answer in time, so this returns null on any failure rather than propagating one.
 */
@OptIn(ExperimentalCoroutinesApi::class)
private suspend fun BugReportViewModel.metricsSnapshot(): String? =
    suspendCancellableCoroutine { cont ->
      Request<String>(
              scope = viewModelScope,
              method = "GET",
              path = "metrics",
              timeoutMillis = 3000,
              responseType = typeOf<String>()) { result ->
                cont.resume(result.getOrNull(), onCancellation = null)
              }
          .execute()
    }

fun BugReportViewModel.exportLogs(context: Context, onResult: (File?) -> Unit) {
  val appContext = context.applicationContext
  viewModelScope.launch {
    val header = metricsSnapshot()
    val file = withContext(Dispatchers.IO) { LogExport.export(appContext, header) }
    onResult(file)
  }
}

fun BugReportViewModel.saveLogsTo(context: Context, uri: Uri, onResult: (Boolean) -> Unit) {
  val appContext = context.applicationContext
  viewModelScope.launch {
    val header = metricsSnapshot()
    val success =
        withContext(Dispatchers.IO) {
          val files = LogExport.bufferFiles(appContext.filesDir)
          if (files.isEmpty()) {
            false
          } else {
            runCatching {
                  appContext.contentResolver.openOutputStream(uri)?.use {
                    LogExport.writeTo(files, it, header)
                  } ?: throw IOException("openOutputStream returned null for $uri")
                }
                .isSuccess
          }
        }
    onResult(success)
  }
}
