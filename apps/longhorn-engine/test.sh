#!/usr/bin/env bash
# Smoke test for a built longhorn-engine image: tools/qw/internal/smoke/servers16.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke longhorn-engine "$@"
