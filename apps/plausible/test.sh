#!/usr/bin/env bash
# Smoke test for a built Plausible image: tools/qw/internal/smoke/plausible.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke plausible "$@"
