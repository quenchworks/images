#!/usr/bin/env bash
# Smoke test for a built sim-realtime image: tools/qw/internal/smoke/sim.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke sim-realtime "$@"
