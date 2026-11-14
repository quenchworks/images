#!/usr/bin/env bash
# Smoke test for a built linkerd-proxy (linkerd2-proxy) image: tools/qw/internal/smoke/servers79.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke linkerd-proxy "$@"
