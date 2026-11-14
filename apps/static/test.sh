#!/usr/bin/env bash
# Smoke test for the built quench-static base: tools/qw/internal/smoke/servers67.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke static "$@"
