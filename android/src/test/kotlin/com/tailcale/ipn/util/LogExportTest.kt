// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package com.tailscale.ipn.util

import android.content.Context
import java.io.ByteArrayOutputStream
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.mockito.kotlin.mock
import org.mockito.kotlin.whenever

class LogExportTest {
  @get:Rule val tmp = TemporaryFolder()

  @Test
  fun `bufferFiles ignores missing and empty files and orders by mtime`() {
    val dataDir = tmp.newFolder("data")
    val older = File(dataDir, "local.log1.txt")
    val newer = File(dataDir, "local.log2.txt")
    older.writeText("2026-09-17T09:00:00.000000Z a\n")
    newer.writeText("2026-09-17T09:00:01.000000Z b\n")
    older.setLastModified(1_000L)
    newer.setLastModified(2_000L)

    val files = LogExport.bufferFiles(dataDir)

    assertEquals(listOf(older, newer), files)
  }

  @Test
  fun `bufferFiles skips files that do not exist or are empty`() {
    val dataDir = tmp.newFolder("data")
    val empty = File(dataDir, "local.log1.txt")
    empty.writeText("")
    // local.log2.txt is never created.

    val files = LogExport.bufferFiles(dataDir)

    assertTrue(files.isEmpty())
  }

  @Test
  fun `bufferFiles does not read logtail's own upload buffer`() {
    // logtail drops lines before buffering when uploads are disabled, so these
    // files are empty exactly when the user has remote logging off. The export
    // must read the locallog ring instead, never fall back to these.
    val dataDir = tmp.newFolder("data")
    File(dataDir, "ipn.log..log1.txt").writeText("{\"logtail\":1}\n")
    File(dataDir, "ipn.log..log2.txt").writeText("{\"logtail\":2}\n")

    val files = LogExport.bufferFiles(dataDir)

    assertTrue(files.isEmpty())
  }

  @Test
  fun `writeTo concatenates files in order with a header per source`() {
    val dataDir = tmp.newFolder("data")
    val first = File(dataDir, "local.log1.txt")
    val second = File(dataDir, "local.log2.txt")
    first.writeText("{\"line\":1}\n")
    second.writeText("{\"line\":2}\n")

    val out = ByteArrayOutputStream()
    LogExport.writeTo(listOf(first, second), out)
    val text = out.toString(Charsets.UTF_8.name())

    val firstHeaderIndex = text.indexOf("# ---- ${first.name} ----")
    val secondHeaderIndex = text.indexOf("# ---- ${second.name} ----")
    assertTrue(firstHeaderIndex >= 0)
    assertTrue(secondHeaderIndex > firstHeaderIndex)
    assertTrue(text.indexOf("{\"line\":1}") in firstHeaderIndex..secondHeaderIndex)
    assertTrue(text.indexOf("{\"line\":2}") > secondHeaderIndex)
  }

  @Test
  fun `export returns null when nothing is buffered`() {
    val dataDir = tmp.newFolder("data")
    val cacheDir = tmp.newFolder("cache")
    val context: Context = mock()
    whenever(context.filesDir).thenReturn(dataDir)
    whenever(context.cacheDir).thenReturn(cacheDir)

    val result = LogExport.export(context)

    assertNull(result)
  }

  @Test
  fun `export cleans up stale exports and writes a new one`() {
    val dataDir = tmp.newFolder("data")
    val cacheDir = tmp.newFolder("cache")
    File(dataDir, "local.log1.txt").writeText("{\"line\":1}\n")
    val logsDir = File(cacheDir, "logs").apply { mkdirs() }
    val stale = File(logsDir, "tailscale-logs-stale.txt").apply { writeText("old") }
    val context: Context = mock()
    whenever(context.filesDir).thenReturn(dataDir)
    whenever(context.cacheDir).thenReturn(cacheDir)

    val result = LogExport.export(context)

    assertTrue(result != null)
    assertTrue(result!!.exists())
    assertTrue(!stale.exists())
    assertTrue(result.readText().contains("{\"line\":1}"))
  }
}
