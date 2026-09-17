// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
package com.tailscale.ipn.util

import android.content.Context
import java.io.File
import java.io.OutputStream
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

// The two files logtail's filch buffer writes under the app data dir, named after the
// "ipn.log." prefix passed to filch.New in libtailscale/tailscale.go.
private val BUFFER_FILE_NAMES = listOf("ipn.log..log1.txt", "ipn.log..log2.txt")

object LogExport {
  fun bufferFiles(dataDir: File): List<File> {
    return BUFFER_FILE_NAMES.map { File(dataDir, it) }
        .filter { it.exists() && it.length() > 0 }
        .sortedBy { it.lastModified() }
  }

  fun writeTo(files: List<File>, out: OutputStream) {
    for (file in files) {
      out.write("# ---- ${file.name} ----\n".toByteArray(Charsets.UTF_8))
      file.inputStream().use { it.copyTo(out) }
    }
  }

  fun export(context: Context): File? {
    val files = bufferFiles(context.filesDir)
    if (files.isEmpty()) return null

    val logsDir = File(context.cacheDir, "logs")
    logsDir.listFiles()?.forEach { it.delete() }
    logsDir.mkdirs()

    val timestamp = SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US).format(Date())
    val out = File(logsDir, "tailscale-logs-$timestamp.txt")
    out.outputStream().use { writeTo(files, it) }
    return out
  }
}
