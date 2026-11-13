#!/usr/bin/env bash
# Smoke test for a built ClickHouse image: tools/qw/internal/smoke/servers51.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke clickhouse "$@"
