#!/usr/bin/env bash
# Smoke test for a built go-httpbin image: tools/qw/internal/smoke/servers117.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke go-httpbin "$@"
