#!/usr/bin/env bash
# Smoke test for a built gotosocial image: tools/qw/internal/smoke/servers121.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke gotosocial "$@"
