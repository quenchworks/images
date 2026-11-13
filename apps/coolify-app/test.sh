#!/usr/bin/env bash
# Smoke test for a built coolify-app image: tools/qw/internal/smoke/servers45.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke coolify-app "$@"
