#!/usr/bin/env bash
# Smoke test for a built PairDrop image: tools/qw/internal/smoke/pairdrop.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke pairdrop "$@"
