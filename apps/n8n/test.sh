#!/usr/bin/env bash
# Smoke test for a built n8n image: tools/qw/internal/smoke/servers36.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke n8n "$@"
