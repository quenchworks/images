#!/usr/bin/env bash
# ============================================================================: tools/qw/internal/smoke/servers73.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke mongodb "$@"
