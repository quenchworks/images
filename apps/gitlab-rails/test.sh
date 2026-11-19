#!/usr/bin/env bash
# Smoke test for a built gitlab-rails image: tools/qw/internal/smoke/gitlab.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke gitlab-rails "$@"
