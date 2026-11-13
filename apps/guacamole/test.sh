#!/usr/bin/env bash
# Smoke test for a built Guacamole web app image: tools/qw/internal/smoke/servers2.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke guacamole "$@"
