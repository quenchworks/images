#!/usr/bin/env bash
# Smoke test for a built LibreSpeed image: tools/qw/internal/smoke/librespeed.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke librespeed "$@"
