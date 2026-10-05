#!/usr/bin/env bash
# Smoke test for a built Policy Reporter image. Usage: test.sh <image-ref> [version]
# Policy Reporter watches PolicyReports in a cluster (the chart's kind gate feeds it a
# real one); here the binary must report its version, find its email templates, and run
# as nonroot.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
out="$(docker run --rm "$IMAGE" version 2>&1)" || { echo "version failed"; echo "$out"; exit 1; }
echo "$out"
[ -z "$WANT" ] || grep -q "AppVersion: $WANT" <<<"$out" || { echo "expected AppVersion: $WANT"; exit 1; }
tpl="$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c 'ls /app/templates' 2>/dev/null || true)"
[ -z "$tpl" ] && tpl="$(docker create "$IMAGE" | { read -r id; docker export "$id" | tar -t | grep '^app/templates/'; docker rm "$id" >/dev/null; })"
grep -q 'summary.html' <<<"$tpl" || { echo "templates missing: $tpl"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (version, templates, nonroot user: $user)"
