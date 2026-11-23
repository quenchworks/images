#!/usr/bin/env bash
# Smoke test for a built IT-Tools (sharevb fork) image: tools/qw/internal/smoke/ittools.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke it-tools-sharevb "$@"
