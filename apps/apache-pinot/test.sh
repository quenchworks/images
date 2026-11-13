#!/usr/bin/env bash
# Smoke test for a built apache-pinot image: tools/qw/internal/smoke/servers14.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke apache-pinot "$@"
