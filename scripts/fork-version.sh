#!/usr/bin/env bash
# Runs the given mkversion command and prints its output plus a VERSION_NAME
# line: the Android versionName, which is VERSION_LONG with a fork revision
# inserted: "1.103.309-t523b626a8-g446b5fee6" gives
# VERSION_NAME="1.103.309.1168-t523b626a8-g446b5fee6", where 1168 is the commit
# count of HEAD.
#
# Upstream's version part only moves when the tailscale.com pin does, so fork
# commits and upstream merges that keep the pin would otherwise differ only in
# the -g<hash>, which an updater comparing versionName can rank lower. The commit
# count grows with every commit and merge. It is joined with "." rather than
# "-": "1.103.309-2-t..." sorts below "1.103.309-t..." ('2' < 't').
#
# VERSION_LONG itself stays exactly as mkversion prints it: version-ldflags.sh
# stamps it into tailscale.com/version.longStamp, and on Android
# version/version_checkformat.go panics at init unless it is X.Y.Z-t<hex>-g<hex>.
set -euo pipefail

rev=$(git rev-list --count HEAD)
out=$("$@")
long=$(sed -nE 's/^VERSION_LONG="(.*)"$/\1/p' <<<"$out")
if ! [[ "$long" =~ ^[0-9]+\.[0-9]+\.[0-9]+-t[0-9a-f]{6,}-g[0-9a-f]{6,}$ ]]; then
    echo >&2 "fork-version.sh: VERSION_LONG \"$long\" is not X.Y.Z-t<hex>-g<hex>; the Go backend would panic at startup"
    exit 1
fi
printf '%s\n' "$out"
echo "VERSION_NAME=\"$(sed -E "s/^([0-9]+\.[0-9]+\.[0-9]+)-/\1.${rev}-/" <<<"$long")\""
