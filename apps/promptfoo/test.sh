#!/usr/bin/env bash
# Smoke test for a built promptfoo image: tools/qw/internal/smoke/promptfoo.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke promptfoo "$@"
