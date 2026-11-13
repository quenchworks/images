#!/usr/bin/env bash
# Smoke test for a built csi-node-driver-registrar image: tools/qw/internal/smoke/servers9.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke csi-node-driver-registrar "$@"
