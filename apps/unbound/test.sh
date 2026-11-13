#!/usr/bin/env bash
# Smoke test for a built unbound image: tools/qw/internal/smoke/servers10.go: tools/qw/internal/smoke/servers10.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke unbound "$@"
