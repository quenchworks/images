#!/usr/bin/env bash
# Smoke test for a built MariaDB Operator image. Usage: test.sh <image-ref> [version]
# The binary carries no version stamp, so this checks it runs as uid 1001 and
# serves every role the chart and the MariaDB pods use (controller, webhook,
# cert-controller, init, agent, backup). The chart gate runs the operator
# against a real MariaDB.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

help="$(docker run --rm --read-only "$IMAGE" --help)"
for cmd in webhook cert-controller init agent backup; do
  grep -qE "^  $cmd " <<<"$help" || { echo "subcommand $cmd missing:"; echo "$help"; exit 1; }
  docker run --rm --read-only "$IMAGE" "$cmd" --help >/dev/null || { echo "$cmd --help failed"; exit 1; }
done
echo "roles: webhook cert-controller init agent backup"

echo "smoke test passed (mariadb-operator ${2:-?}: all roles present; nonroot user: $user)"
