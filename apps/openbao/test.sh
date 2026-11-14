#!/usr/bin/env bash
# Smoke test for a built OpenBao image: tools/qw/internal/smoke/servers76.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke openbao "$@"
