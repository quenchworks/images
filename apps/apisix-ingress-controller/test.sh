#!/usr/bin/env bash
# Boot test for a built APISIX Ingress Controller image.: tools/qw/internal/smoke/servers83.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke apisix-ingress-controller "$@"
