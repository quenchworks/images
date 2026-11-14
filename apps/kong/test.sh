#!/usr/bin/env bash
# Smoke test for a built Kong Gateway image: tools/qw/internal/smoke/servers82.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke kong "$@"
