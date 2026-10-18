#!/usr/bin/env bash
# Builds libv2ray.aar from libv2scale (Xray core wrapper) with gomobile and copies it to
# V2rayNG/app/libs/. Default target: android/arm64 only; pass TARGETS=android for all ABIs.
set -euo pipefail
source "$(dirname "$0")/env.sh"
cd "$ROOT/libv2scale"
mkdir -p assets data
if [ ! -s assets/geoip.dat ] || [ ! -s assets/geosite.dat ]; then
  bash gen_assets.sh download
  cp -v data/*.dat assets/
fi
gomobile init
gomobile bind -v -androidapi 24 -trimpath -target="${TARGETS:-android/arm64}" \
  -ldflags='-s -w -buildid= -checklinkname=0' ./
mkdir -p "$ROOT/V2rayNG/app/libs"
cp libv2ray.aar "$ROOT/V2rayNG/app/libs/"
ls -la "$ROOT/V2rayNG/app/libs"
