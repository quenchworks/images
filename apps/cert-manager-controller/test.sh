#!/usr/bin/env bash
# Smoke test for a built cert-manager controller image: tools/qw/internal/smoke/simple.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke cert-manager-controller "$@"
