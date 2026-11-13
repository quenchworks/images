#!/usr/bin/env bash
# Smoke test for the Concourse git resource image: tools/qw/internal/smoke/servers9.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke concourse-git-resource "$@"
