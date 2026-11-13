#!/usr/bin/env bash
# Smoke test for a built dnscrypt-proxy image: tools/qw/internal/smoke/servers38.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke dnscrypt-proxy "$@"
