#!/usr/bin/env bash
# Smoke test for a built Knative Operator image: tools/qw/internal/smoke/servers56.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke knative-operator "$@"
