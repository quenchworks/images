#!/usr/bin/env bash
# Smoke test for a built selenium-server image: tools/qw/internal/smoke/servers12.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke selenium-server "$@"
