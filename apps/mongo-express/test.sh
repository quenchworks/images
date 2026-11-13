#!/usr/bin/env bash
# Smoke test for a built mongo-express image: tools/qw/internal/smoke/servers34.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke mongo-express "$@"
