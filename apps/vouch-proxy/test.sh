#!/usr/bin/env bash
# Smoke test for a built Vouch Proxy image: tools/qw/internal/smoke/servers11.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke vouch-proxy "$@"
