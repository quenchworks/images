#!/usr/bin/env bash
# Smoke test for a built Temporal UI image: tools/qw/internal/smoke/servers58.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke temporal-ui "$@"
