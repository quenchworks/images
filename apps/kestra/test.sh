#!/usr/bin/env bash
# Smoke test for a built Kestra image: tools/qw/internal/smoke/kestra.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke kestra "$@"
