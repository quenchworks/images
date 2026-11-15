#!/usr/bin/env bash
# Smoke test for a built gitlab-shell image: tools/qw/internal/smoke/servers103.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke gitlab-shell "$@"
