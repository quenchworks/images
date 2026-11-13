#!/usr/bin/env bash
# Smoke test for a built Homebox image: tools/qw/internal/smoke/servers29.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke homebox "$@"
