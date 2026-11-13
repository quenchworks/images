#!/usr/bin/env bash
# Smoke test for a built MariaDB Operator image: tools/qw/internal/smoke/tools.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke mariadb-operator "$@"
