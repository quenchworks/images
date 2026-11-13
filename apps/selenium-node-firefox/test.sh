#!/usr/bin/env bash
# Smoke test for a built selenium-node-firefox image: tools/qw/internal/smoke/servers15.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke selenium-node-firefox "$@"
