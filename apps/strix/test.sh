#!/usr/bin/env bash
# Smoke test for a built Strix image: tools/qw/internal/smoke/strix.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke strix "$@"
