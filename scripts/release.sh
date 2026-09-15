#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
make apk
apksigner sign --ks ../my-debug.jks --ks-key-alias my-key --ks-pass "pass:$1" \
  --v1-signing-enabled false --v2-signing-enabled true --v3-signing-enabled false \
  --out "$(pwd)/my-app-signed.apk" tailscale-debug.apk
