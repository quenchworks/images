#!/usr/bin/env bash
# Smoke test for a built nginx-proxy-manager image: tools/qw/internal/smoke/servers59.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke nginx-proxy-manager "$@"
