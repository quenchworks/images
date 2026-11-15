#!/usr/bin/env bash
# Smoke test for a built dufs image: tools/qw/internal/smoke/servers101.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke dufs "$@"
