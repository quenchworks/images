#!/usr/bin/env bash
# Boot test for a built ADC image.: tools/qw/internal/smoke/servers80.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke adc "$@"
