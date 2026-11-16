#!/usr/bin/env bash
# Smoke test for a built silverbullet image: tools/qw/internal/smoke/servers148.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke silverbullet "$@"
