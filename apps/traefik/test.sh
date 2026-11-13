#!/usr/bin/env bash
# Smoke test for a built Traefik image: tools/qw/internal/smoke/servers23.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke traefik "$@"
