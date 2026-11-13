#!/usr/bin/env bash
# Smoke test for a built cass-operator image: tools/qw/internal/smoke/servers53.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke cass-operator "$@"
