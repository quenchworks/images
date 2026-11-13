#!/usr/bin/env bash
# Smoke test for the Zabbix server image: tools/qw/internal/smoke/servers30.go. Usage: test.sh <image-ref> [version]
exec go -C "$(cd "$(dirname "$0")/../../tools/qw" && pwd)" run . smoke zabbix-server "$@"
