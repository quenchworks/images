#!/usr/bin/env bash
# Smoke test for a built calibre-web image: tools/qw/internal/smoke/servers158.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke calibre-web "$@"
