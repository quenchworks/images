#!/usr/bin/env bash
# Smoke test for a built Cassandra image: tools/qw/internal/smoke/servers52.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke cassandra "$@"
