#!/usr/bin/env bash
# Smoke test for a built Hoppscotch backend image: tools/qw/internal/smoke/hoppscotch.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke hoppscotch-backend "$@"
