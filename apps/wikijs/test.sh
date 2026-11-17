#!/usr/bin/env bash
# Smoke test for a built wikijs image: tools/qw/internal/smoke/servers157.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke wikijs "$@"
