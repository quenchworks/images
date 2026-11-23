#!/usr/bin/env bash
# Smoke test for a built Traccar image: tools/qw/internal/smoke/traccar.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke traccar "$@"
