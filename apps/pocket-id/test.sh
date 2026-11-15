#!/usr/bin/env bash
# Smoke test for a built Pocket ID image: tools/qw/internal/smoke/servers87.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke pocket-id "$@"
