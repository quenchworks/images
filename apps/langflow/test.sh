#!/usr/bin/env bash
# Smoke test for a built Langflow image: tools/qw/internal/smoke/langflow.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke langflow "$@"
