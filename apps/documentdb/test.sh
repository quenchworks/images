#!/usr/bin/env bash
# Smoke test for the standalone DocumentDB image: tools/qw/internal/smoke/servers45.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke documentdb "$@"
