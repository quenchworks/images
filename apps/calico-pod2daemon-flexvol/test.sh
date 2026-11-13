#!/usr/bin/env bash
# Smoke test for a built calico-pod2daemon-flexvol image: tools/qw/internal/smoke/simple.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke calico-pod2daemon-flexvol "$@"
