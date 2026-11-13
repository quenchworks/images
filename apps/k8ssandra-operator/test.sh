#!/usr/bin/env bash
# Smoke test for a built K8ssandra Operator image: tools/qw/internal/smoke/servers53.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke k8ssandra-operator "$@"
