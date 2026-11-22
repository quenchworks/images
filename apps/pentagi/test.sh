#!/usr/bin/env bash
# Smoke test for a built pentagi image: tools/qw/internal/smoke/pentagi.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke pentagi "$@"
