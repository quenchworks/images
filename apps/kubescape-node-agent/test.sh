#!/usr/bin/env bash
# Smoke test for a built kubescape-node-agent image: tools/qw/internal/smoke/servers13.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke kubescape-node-agent "$@"
