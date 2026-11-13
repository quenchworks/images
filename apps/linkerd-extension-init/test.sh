#!/usr/bin/env bash
# Smoke test for a built linkerd-extension-init image: tools/qw/internal/smoke/servers32.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke linkerd-extension-init "$@"
