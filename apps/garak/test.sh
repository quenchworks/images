#!/usr/bin/env bash
# Smoke test for a built garak image: tools/qw/internal/smoke/garak.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke garak "$@"
