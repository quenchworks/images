#!/usr/bin/env bash
# Smoke test for a built NGINX Gateway Fabric control-plane image.: tools/qw/internal/smoke/servers14.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke nginx-gateway-fabric "$@"
