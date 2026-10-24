#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
# tailscale.version's make deps don't change on a commit; regenerate it so the
# APK is stamped with HEAD and its fork revision (scripts/fork-version.sh).
rm -f tailscale.version
make libtailscale debug-symbols version build-unstripped-aar
(cd android && ./gradlew assembleRelease)
apksigner sign --ks ../my-debug.jks --ks-key-alias my-key --ks-pass "pass:$1" \
  --v1-signing-enabled false --v2-signing-enabled true --v3-signing-enabled false \
  --out "$(pwd)/my-app-signed.apk" android/build/outputs/apk/release/android-release-unsigned.apk
source tailscale.version && echo "my-app-signed.apk: ${VERSION_LONG}"
