#!/usr/bin/env bash
# wsltest.sh — cross-compile a package's tests for linux on Windows and run them
# inside the Debian WSL distro. The Go source is largely //go:build linux, so the
# native windows/amd64 toolchain cannot run these tests; this compiles a linux
# test binary and executes it in WSL (which has no Go toolchain of its own).
#
# Usage: scripts/wsltest.sh ./internal/vm [-test.run TestFoo] [-test.v]
set -euo pipefail

pkg="${1:?usage: wsltest.sh <package> [extra test flags...]}"
shift || true

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out_dir="$repo_root/.testbin"
mkdir -p "$out_dir"
# Derive a stable, filesystem-safe binary name from the package path.
bin_name="$(echo "$pkg" | sed 's#[./]#_#g').test"
out_bin="$out_dir/$bin_name"

echo ">> cross-compiling $pkg tests for linux/amd64"
GOOS=linux GOARCH=amd64 go test -c -o "$out_bin" "$pkg"

wsl_bin="$(wsl.exe -d Debian -- wslpath "$(cygpath -w "$out_bin" 2>/dev/null || echo "$out_bin")" 2>/dev/null | tr -d '\0\r')"
echo ">> running in WSL: $wsl_bin $*"
wsl.exe -d Debian -- bash -lc "chmod +x '$wsl_bin' && '$wsl_bin' -test.count=1 $* " 2>&1 | tr -d '\0'
