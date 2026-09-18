// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
package com.tailscale.ipn.util

import android.content.Context
import java.io.File
import java.io.OutputStream
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

// The two ring files libtailscale/locallog writes under the app data dir; mirrors
// locallog.FileNames() in Go. Not logtail's own "ipn.log.*" buffer: that one is empty
// whenever remote logging is off, because logtail drops lines before buffering them.
private val BUFFER_FILE_NAMES = listOf("local.log1.txt", "local.log2.txt")

object LogExport {
  fun bufferFiles(dataDir: File): List<File> {
    return BUFFER_FILE_NAMES.map { File(dataDir, it) }
        .filter { it.exists() && it.length() > 0 }
        .sortedBy { it.lastModified() }
  }

  /**
   * Writes files to out, each preceded by a source header. When header is non-null (a
   * /localapi/v0/metrics snapshot taken at export time), it is written first under its own header,
   * so the absolute counters at export time are available to cross-check the metricslog deltas in
   * the ring files that follow.
   */
  fun writeTo(files: List<File>, out: OutputStream, header: String? = null) {
    if (header != null) {
      out.write("# ---- metrics snapshot ----\n".toByteArray(Charsets.UTF_8))
      out.write(header.toByteArray(Charsets.UTF_8))
      if (!header.endsWith("\n")) out.write("\n".toByteArray(Charsets.UTF_8))
    }
    for (file in files) {
      out.write("# ---- ${file.name} ----\n".toByteArray(Charsets.UTF_8))
      file.inputStream().use { it.copyTo(out) }
    }
  }

  fun export(context: Context, header: String? = null): File? {
    val files = bufferFiles(context.filesDir)
    if (files.isEmpty()) return null

    val logsDir = File(context.cacheDir, "logs")
    logsDir.listFiles()?.forEach { it.delete() }
    logsDir.mkdirs()

    val timestamp = SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US).format(Date())
    val out = File(logsDir, "tailscale-logs-$timestamp.txt")
    out.outputStream().use { writeTo(files, it, header) }
    return out
  }
}
