#!/usr/bin/env bash
# Smoke test for a built Cosign image: tools/qw/internal/smoke/cosign.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke cosign "$@"
