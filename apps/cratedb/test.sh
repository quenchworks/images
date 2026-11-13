#!/usr/bin/env bash
# Smoke test for a built CrateDB image: tools/qw/internal/smoke/servers25.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke cratedb "$@"
