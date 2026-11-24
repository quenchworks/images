#!/usr/bin/env bash
# Smoke test for a built Falcosidekick image: tools/qw/internal/smoke/falcosidekick.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke falcosidekick "$@"
