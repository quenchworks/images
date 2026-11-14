#!/usr/bin/env bash
# Smoke test for a built Volcano image: tools/qw/internal/smoke/servers55.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke volcano "$@"
