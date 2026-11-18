#!/usr/bin/env bash
# Smoke test for a built spilo image: tools/qw/internal/smoke/spilo.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke spilo "$@"
