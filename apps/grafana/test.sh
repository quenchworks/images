#!/usr/bin/env bash
# Smoke test for a built Grafana image: tools/qw/internal/smoke/servers33.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke grafana "$@"
