#!/usr/bin/env bash
# Smoke test for a built istio-proxyv2 image: tools/qw/internal/smoke/servers20.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke istio-proxyv2 "$@"
