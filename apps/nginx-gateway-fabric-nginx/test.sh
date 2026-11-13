#!/usr/bin/env bash
# Smoke test for a built NGINX Gateway Fabric data-plane image.: tools/qw/internal/smoke/servers31.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke nginx-gateway-fabric-nginx "$@"
