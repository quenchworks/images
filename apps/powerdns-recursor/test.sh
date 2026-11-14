#!/usr/bin/env bash
# Smoke test for a built PowerDNS Recursor image: tools/qw/internal/smoke/servers66.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke powerdns-recursor "$@"
