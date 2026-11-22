#!/usr/bin/env bash
# Smoke test for a built Nuclei image: tools/qw/internal/smoke/nuclei.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke nuclei "$@"
