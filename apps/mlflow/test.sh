#!/usr/bin/env bash
# Smoke test for a built MLflow Tracking Server image: tools/qw/internal/smoke/servers39.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke mlflow "$@"
