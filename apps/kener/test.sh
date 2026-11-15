#!/usr/bin/env bash
# Smoke test for a built kener image: tools/qw/internal/smoke/servers120.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke kener "$@"
