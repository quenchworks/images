#!/usr/bin/env bash
# Smoke test for a built camel-k image: tools/qw/internal/smoke/servers27.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke camel-k "$@"
