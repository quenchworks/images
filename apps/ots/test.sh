#!/usr/bin/env bash
# Smoke test for a built OTS image: tools/qw/internal/smoke/ots.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke ots "$@"
