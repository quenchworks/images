#!/usr/bin/env bash
# Smoke test for a built cyberchef image: tools/qw/internal/smoke/servers118.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke cyberchef "$@"
