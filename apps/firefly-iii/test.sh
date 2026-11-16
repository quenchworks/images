#!/usr/bin/env bash
# Smoke test for a built firefly-iii image: tools/qw/internal/smoke/servers132.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke firefly-iii "$@"
