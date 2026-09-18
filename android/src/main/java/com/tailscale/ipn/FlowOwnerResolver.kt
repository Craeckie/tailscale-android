// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
package com.tailscale.ipn

import android.content.Context
import android.net.ConnectivityManager
import android.os.Build
import android.os.Process
import java.net.InetAddress
import java.net.InetSocketAddress
import java.util.concurrent.ConcurrentHashMap
import libtailscale.FlowOwnerLookup

/**
 * Resolves the Android app behind a tunnel flow via ConnectivityManager.getConnectionOwnerUid, so
 * the flow log (libtailscale/flowlog) can attribute tunnel traffic to a package without adb.
 */
class FlowOwnerResolver(context: Context) : FlowOwnerLookup {
  private val appContext = context.applicationContext
  private val connectivityManager =
      appContext.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
  private val packageCache = ConcurrentHashMap<Int, String>()

  override fun flowOwner(
      proto: Int,
      srcIP: String,
      srcPort: Int,
      dstIP: String,
      dstPort: Int
  ): String {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return ""
    return try {
      val local = InetSocketAddress(InetAddress.getByName(srcIP), srcPort)
      val remote = InetSocketAddress(InetAddress.getByName(dstIP), dstPort)
      val uid = connectivityManager.getConnectionOwnerUid(proto, local, remote)
      if (uid == Process.INVALID_UID) "" else packageCache.getOrPut(uid) { resolvePackages(uid) }
    } catch (e: Exception) {
      ""
    }
  }

  private fun resolvePackages(uid: Int): String {
    val names =
        try {
          appContext.packageManager.getPackagesForUid(uid)
        } catch (e: SecurityException) {
          null
        }
    return if (names.isNullOrEmpty()) "uid:$uid" else names.joinToString(",")
  }
}
