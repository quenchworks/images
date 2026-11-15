#!/usr/bin/env bash
# Smoke test for a built gitlab-workhorse image: tools/qw/internal/smoke/servers104.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke gitlab-workhorse "$@"
