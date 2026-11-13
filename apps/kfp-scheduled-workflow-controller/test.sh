#!/usr/bin/env bash
# Smoke test for a built kfp-scheduled-workflow-controller image: tools/qw/internal/smoke/simple.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke kfp-scheduled-workflow-controller "$@"
