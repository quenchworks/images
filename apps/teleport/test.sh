#!/usr/bin/env bash
# Boot test for the Teleport image: tools/qw/internal/smoke/servers49.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke teleport "$@"
