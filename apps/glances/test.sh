#!/usr/bin/env bash
# Smoke test for a built Glances image: tools/qw/internal/smoke/glances.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke glances "$@"
