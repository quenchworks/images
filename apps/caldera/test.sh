#!/usr/bin/env bash
# Smoke test for a built MITRE Caldera image: tools/qw/internal/smoke/caldera.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke caldera "$@"
