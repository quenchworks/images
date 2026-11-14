#!/usr/bin/env bash
# Smoke test for a built kgateway data-plane image (upstream's `envoy-wrapper`).: tools/qw/internal/smoke/servers81.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke kgateway-envoy "$@"
