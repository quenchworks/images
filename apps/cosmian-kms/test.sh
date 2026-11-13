#!/usr/bin/env bash
# Smoke test for a built Cosmian KMS image: tools/qw/internal/smoke/servers44.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke cosmian-kms "$@"
