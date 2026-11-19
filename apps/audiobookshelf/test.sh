#!/usr/bin/env bash
# Smoke test for a built audiobookshelf image: tools/qw/internal/smoke/audiobookshelf.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke audiobookshelf "$@"
