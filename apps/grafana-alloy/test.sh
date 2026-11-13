#!/usr/bin/env bash
# Smoke test for a built Grafana Alloy image: tools/qw/internal/smoke/servers28.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke grafana-alloy "$@"
