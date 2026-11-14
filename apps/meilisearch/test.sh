#!/usr/bin/env bash
# Smoke test for a built Meilisearch image: tools/qw/internal/smoke/servers76.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke meilisearch "$@"
