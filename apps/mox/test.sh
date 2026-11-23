#!/usr/bin/env bash
# Smoke test for a built mox image: tools/qw/internal/smoke/mox.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke mox "$@"
