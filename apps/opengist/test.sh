#!/usr/bin/env bash
# Smoke test for a built Opengist image: tools/qw/internal/smoke/servers89.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke opengist "$@"
