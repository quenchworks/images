#!/usr/bin/env bash
# Smoke test for a built KubeVirt CDI operator image: tools/qw/internal/smoke/servers60.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke cdi-operator "$@"
