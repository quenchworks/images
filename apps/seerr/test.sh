#!/usr/bin/env bash
# Smoke test for a built seerr image: tools/qw/internal/smoke/seerr.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke seerr "$@"
