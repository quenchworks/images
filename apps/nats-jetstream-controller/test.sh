#!/usr/bin/env bash
# Smoke test for the NATS JetStream Controller image: tools/qw/internal/smoke/simple.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke nats-jetstream-controller "$@"
