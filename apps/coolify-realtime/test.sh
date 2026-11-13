#!/usr/bin/env bash
# Smoke test for a built Coolify realtime image: tools/qw/internal/smoke/servers35.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke coolify-realtime "$@"
