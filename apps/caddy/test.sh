#!/usr/bin/env bash
# Smoke test for a built Caddy image: tools/qw/internal/smoke/servers27.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke caddy "$@"
