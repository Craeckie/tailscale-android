// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package libtailscale

import (
	"net/netip"
	"sync/atomic"

	"tailscale.com/net/tstun"
	"tailscale.com/types/ipproto"
	"tailscale.com/types/logger"

	"github.com/tailscale/tailscale-android/libtailscale/flowlog"
)

// FlowOwnerLookup resolves the Android app owning a tunnel flow. Kotlin
// implements this via ConnectivityManager.getConnectionOwnerUid +
// PackageManager.getPackagesForUid.
type FlowOwnerLookup interface {
	// FlowOwner returns the package name(s) owning the flow identified by
	// proto (an IP protocol number, e.g. 6 for TCP or 17 for UDP), the local
	// (src) and remote (dst) endpoints, or "" if unknown.
	FlowOwner(proto int32, srcIP string, srcPort int32, dstIP string, dstPort int32) string
}

var flowOwnerLookup atomic.Pointer[FlowOwnerLookup]

// SetFlowOwnerLookup registers the Kotlin-side flow owner resolver. It must
// be called before the tunnel starts for early flows to be attributed.
func SetFlowOwnerLookup(l FlowOwnerLookup) {
	flowOwnerLookup.Store(&l)
}

// installFlowLog wires a flowlog.Logger into w, resolving each new flow's
// owner through whatever FlowOwnerLookup Kotlin has registered (if any).
func installFlowLog(w *tstun.Wrapper, logf logger.Logf) {
	owner := func(proto ipproto.Proto, src, dst netip.AddrPort) string {
		p := flowOwnerLookup.Load()
		if p == nil {
			return ""
		}
		return (*p).FlowOwner(int32(proto), src.Addr().String(), int32(src.Port()), dst.Addr().String(), int32(dst.Port()))
	}
	flowlog.New(logf, owner).Install(w)
}
