#!/usr/bin/env bash
# Smoke test for a built ScyllaDB Operator image: tools/qw/internal/smoke/scylla_operator.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke scylla-operator "$@"
