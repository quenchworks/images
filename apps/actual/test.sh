#!/usr/bin/env bash
# Smoke test for a built Actual image: tools/qw/internal/smoke/servers4.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke actual "$@"
