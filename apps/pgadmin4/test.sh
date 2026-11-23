#!/usr/bin/env bash
# Smoke test for a built pgAdmin 4 image: tools/qw/internal/smoke/pgadmin4.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke pgadmin4 "$@"
