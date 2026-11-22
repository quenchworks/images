#!/usr/bin/env bash
# Smoke test for a built ZAP image: tools/qw/internal/smoke/zap.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke zap "$@"
