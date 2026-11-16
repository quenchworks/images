#!/usr/bin/env bash
# Smoke test for a built chhoto-url image: tools/qw/internal/smoke/servers144.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke chhoto-url "$@"
