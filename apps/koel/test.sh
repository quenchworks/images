#!/usr/bin/env bash
# Smoke test for a built koel image: tools/qw/internal/smoke/servers135.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke koel "$@"
