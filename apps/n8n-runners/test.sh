#!/usr/bin/env bash
# Smoke test for a built n8n-runners image: tools/qw/internal/smoke/servers9.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke n8n-runners "$@"
