#!/usr/bin/env bash
# Smoke test for a built code-server image: tools/qw/internal/smoke/servers21.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke code-server "$@"
