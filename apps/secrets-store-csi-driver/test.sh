#!/usr/bin/env bash
# Smoke test for a built secrets-store-csi-driver image: tools/qw/internal/smoke/simple.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke secrets-store-csi-driver "$@"
