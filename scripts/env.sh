#!/usr/bin/env bash
# Toolchain locations for this machine. Source it, do not run it.
# Override any variable from the environment for another layout.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export ROOT
export JAVA_HOME="${JAVA_HOME_V2SCALE:-/opt/homebrew/opt/openjdk@21}"
export ANDROID_HOME="${ANDROID_HOME:-/opt/homebrew/share/android-commandlinetools}"
export ANDROID_SDK_ROOT="$ANDROID_HOME"
export NDK_HOME="${NDK_HOME:-$ANDROID_HOME/ndk/30.0.16248370}"
export ANDROID_NDK_HOME="$NDK_HOME"
export PATH="$JAVA_HOME/bin:$HOME/go/bin:$PATH"
