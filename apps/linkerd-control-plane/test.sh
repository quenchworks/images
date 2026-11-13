#!/usr/bin/env bash
# Smoke test for a built linkerd-control-plane image: tools/qw/internal/smoke/servers10.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke linkerd-control-plane "$@"
