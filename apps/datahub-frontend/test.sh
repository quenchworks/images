#!/usr/bin/env bash
# Smoke test for a built datahub-frontend image: tools/qw/internal/smoke/servers69.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke datahub-frontend "$@"
