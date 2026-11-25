#!/usr/bin/env bash
# Smoke test for a built HAProxy Kubernetes Ingress image: tools/qw/internal/smoke/haproxyingress.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke haproxy-kubernetes-ingress "$@"
