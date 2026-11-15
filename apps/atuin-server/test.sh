#!/usr/bin/env bash
# Smoke test for a built atuin-server image: tools/qw/internal/smoke/servers119.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke atuin-server "$@"
