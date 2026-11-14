#!/usr/bin/env bash
# Smoke test for the Strimzi Kafka image: tools/qw/internal/smoke/servers78.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke strimzi-kafka "$@"
