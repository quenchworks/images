#!/usr/bin/env bash
# Smoke test for a built BusyBox toolbox image: tools/qw/internal/smoke/servers7.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke busybox "$@"
