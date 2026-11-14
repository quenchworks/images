#!/usr/bin/env bash
# Smoke test for the built Harbor Trivy scanner-adapter image.: tools/qw/internal/smoke/servers77.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke harbor-trivy-adapter "$@"
