#!/usr/bin/env bash
# Smoke test for a built linkerd-viz-metrics-api image: tools/qw/internal/smoke/servers31.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke linkerd-viz-metrics-api "$@"
