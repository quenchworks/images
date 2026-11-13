#!/usr/bin/env bash
# Smoke test for a built quench-dotnet-runtime image.: tools/qw/internal/smoke/servers35.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke dotnet-runtime "$@"
