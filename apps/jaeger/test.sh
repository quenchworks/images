#!/usr/bin/env bash
# Smoke test for a built Jaeger v2 image: tools/qw/internal/smoke/servers5.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke jaeger "$@"
