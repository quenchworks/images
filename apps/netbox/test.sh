#!/usr/bin/env bash
# Smoke test for a built NetBox image: tools/qw/internal/smoke/netbox.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke netbox "$@"
