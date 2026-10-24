#!/usr/bin/env bash
# Runs the given mkversion command and prints its output with a fork revision
# inserted into VERSION_LONG: "1.103.309-t523b626a8-g446b5fee6" becomes
# "1.103.309.1168-t523b626a8-g446b5fee6", where 1168 is the commit count of HEAD.
#
# Upstream's version part only moves when the tailscale.com pin does, so fork
# commits and upstream merges that keep the pin would otherwise differ only in
# the -g<hash>, which an updater comparing versionName can rank lower. The commit
# count grows with every commit and merge. It is joined with "." rather than
# "-": "1.103.309-2-t..." sorts below "1.103.309-t..." ('2' < 't').
#
# VERSION_SHORT is left alone, since tailscale.com/version parses it.
set -euo pipefail

rev=$(git rev-list --count HEAD)
"$@" | sed -E "s/^(VERSION_LONG=\"[0-9]+\.[0-9]+\.[0-9]+)-/\1.${rev}-/"
