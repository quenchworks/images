#!/usr/bin/env bash
# Boot test for a built kfp-frontend image: tools/qw/internal/smoke/servers13.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke kfp-frontend "$@"
