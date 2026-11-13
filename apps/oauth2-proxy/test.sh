#!/usr/bin/env bash
# Smoke test for a built oauth2-proxy image: tools/qw/internal/smoke/servers.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke oauth2-proxy "$@"
