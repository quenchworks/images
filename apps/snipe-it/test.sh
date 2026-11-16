#!/usr/bin/env bash
# Smoke test for a built snipe-it image: tools/qw/internal/smoke/servers134.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke snipe-it "$@"
