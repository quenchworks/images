#!/usr/bin/env bash
# Smoke test for a built kgateway control-plane image.: tools/qw/internal/smoke/servers18.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke kgateway "$@"
