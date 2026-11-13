#!/usr/bin/env bash
# Smoke test for a built Ansible control-node image: tools/qw/internal/smoke/servers30.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke ansible "$@"
