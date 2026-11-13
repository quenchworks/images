#!/usr/bin/env bash
# Smoke test for a built OPA image: tools/qw/internal/smoke/opa.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke opa "$@"
