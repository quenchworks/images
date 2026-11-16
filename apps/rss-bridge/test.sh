#!/usr/bin/env bash
# Smoke test for a built rss-bridge image: tools/qw/internal/smoke/servers143.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke rss-bridge "$@"
