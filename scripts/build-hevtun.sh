#!/usr/bin/env bash
# Builds libhev-socks5-tunnel for the chosen ABIs (default: arm64-v8a only, enough for
# development; pass HEV_ABIS="armeabi-v7a arm64-v8a x86 x86_64" for a full set) using the
# unmodified upstream compile-hevtun.sh. Output: libs/<abi>/*.so, copied to V2rayNG/app/libs/.
set -euo pipefail
source "$(dirname "$0")/env.sh"
cd "$ROOT"
sed "s/^ABIS=.*/ABIS=\"${HEV_ABIS:-arm64-v8a}\"/" compile-hevtun.sh > .compile-hevtun.dev.sh
trap 'rm -f "$ROOT/.compile-hevtun.dev.sh"' EXIT
bash .compile-hevtun.dev.sh
mkdir -p V2rayNG/app/libs
cp -r libs/* V2rayNG/app/libs/
ls V2rayNG/app/libs
