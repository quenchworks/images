#!/usr/bin/env bash
# Smoke test for a built CommaFeed image: tools/qw/internal/smoke/commafeed.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke commafeed "$@"
