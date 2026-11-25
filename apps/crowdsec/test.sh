#!/usr/bin/env bash
# Smoke test for a built CrowdSec image: tools/qw/internal/smoke/crowdsec.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke crowdsec "$@"
