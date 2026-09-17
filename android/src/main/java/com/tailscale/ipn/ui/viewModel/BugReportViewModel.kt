// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package com.tailscale.ipn.ui.viewModel

import android.content.Context
import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.tailscale.ipn.ui.localapi.Client
import com.tailscale.ipn.ui.util.set
import com.tailscale.ipn.util.LogExport
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class BugReportViewModel : ViewModel() {
  val bugReportID: StateFlow<String> = MutableStateFlow("")

  init {
    Client(viewModelScope).bugReportId { result ->
      result
          .onSuccess { bugReportID.set(it.trim()) }
          .onFailure { bugReportID.set("(Error fetching ID)") }
    }
  }

  fun exportLogs(context: Context, onResult: (File?) -> Unit) {
    val appContext = context.applicationContext
    viewModelScope.launch {
      val file = withContext(Dispatchers.IO) { LogExport.export(appContext) }
      onResult(file)
    }
  }

  fun saveLogsTo(context: Context, uri: Uri, onResult: (Boolean) -> Unit) {
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
                    } ?: throw java.io.IOException("openOutputStream returned null for $uri")
                  }
                  .isSuccess
            }
          }
      onResult(success)
    }
  }
}
