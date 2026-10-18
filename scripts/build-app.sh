#!/usr/bin/env bash
# Assembles the debug APK. Needs V2rayNG/app/libs/libv2ray.aar and libhev*.so
# (run scripts/build-core.sh and scripts/build-hevtun.sh first).
# Usage: scripts/build-app.sh [gradle task]   (default: assemblePlaystoreDebug)
set -euo pipefail
source "$(dirname "$0")/env.sh"
cd "$ROOT/V2rayNG"
echo "sdk.dir=$ANDROID_HOME" > local.properties
./gradlew "${1:-assemblePlaystoreDebug}" -PABI_FILTERS="${ABI_FILTERS:-arm64-v8a}"
find app/build/outputs/apk -name '*.apk' -print
