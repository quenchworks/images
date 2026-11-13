#!/usr/bin/env bash
# Smoke test for a built PocketBase image: tools/qw/internal/smoke/simple.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke pocketbase "$@"
