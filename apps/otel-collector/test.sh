#!/usr/bin/env bash
# Smoke test for a built OpenTelemetry Collector image: tools/qw/internal/smoke/servers68.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke otel-collector "$@"
