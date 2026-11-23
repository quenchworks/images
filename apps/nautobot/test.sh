#!/usr/bin/env bash
# Smoke test for a built Nautobot image: tools/qw/internal/smoke/nautobot.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke nautobot "$@"
