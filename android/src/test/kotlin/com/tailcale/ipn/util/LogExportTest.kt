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
    val older = File(dataDir, "ipn.log..log1.txt")
    val newer = File(dataDir, "ipn.log..log2.txt")
    older.writeText("{\"a\":1}\n")
    newer.writeText("{\"b\":2}\n")
    older.setLastModified(1_000L)
    newer.setLastModified(2_000L)

    val files = LogExport.bufferFiles(dataDir)

    assertEquals(listOf(older, newer), files)
  }

  @Test
  fun `bufferFiles skips files that do not exist or are empty`() {
    val dataDir = tmp.newFolder("data")
    val empty = File(dataDir, "ipn.log..log1.txt")
    empty.writeText("")
    // ipn.log..log2.txt is never created.

    val files = LogExport.bufferFiles(dataDir)

    assertTrue(files.isEmpty())
  }

  @Test
  fun `writeTo concatenates files in order with a header per source`() {
    val dataDir = tmp.newFolder("data")
    val first = File(dataDir, "ipn.log..log1.txt")
    val second = File(dataDir, "ipn.log..log2.txt")
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
    File(dataDir, "ipn.log..log1.txt").writeText("{\"line\":1}\n")
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
