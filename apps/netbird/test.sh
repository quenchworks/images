#!/usr/bin/env bash
# Smoke test for a built netbird image: tools/qw/internal/smoke/netbird.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke netbird "$@"
