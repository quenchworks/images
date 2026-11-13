#!/usr/bin/env bash
# Smoke test for a built Blocky image: tools/qw/internal/smoke/servers50.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke blocky "$@"
