#!/usr/bin/env bash
# Smoke test for a built YARA image: tools/qw/internal/smoke/yara.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke yara "$@"
