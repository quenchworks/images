#!/usr/bin/env bash
# Smoke test for a built leantime image: tools/qw/internal/smoke/leantime.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke leantime "$@"
