#!/usr/bin/env bash
# Smoke test for a built yq image: tools/qw/internal/smoke/servers122.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke yq "$@"
