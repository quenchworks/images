#!/usr/bin/env bash
# Smoke test for a built calico-kube-controllers image. Usage: test.sh <image-ref> [version]
# Checks the user and that the binaries report the release. The controllers themselves
# need a cluster: the Calico chart's kind gate runs them.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
out="$(docker run --rm --entrypoint /usr/bin/calico-kube-controllers "$IMAGE" -version 2>&1 || true)"
echo "/usr/bin/calico-kube-controllers -version: $out"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$out" || { echo "/usr/bin/calico-kube-controllers: expected v$WANT"; exit 1; }
out="$(docker run --rm --entrypoint /usr/bin/check-status "$IMAGE" -v 2>&1 || true)"
echo "/usr/bin/check-status -v: $out"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$out" || { echo "/usr/bin/check-status: expected v$WANT"; exit 1; }
# The operator runs this image as uid 999, gid 0; its probes read /status/status.json.
cid="$(docker create "$IMAGE")"
st="$(docker export "$cid" | tar -tvf - --numeric-owner status/status.json)"; docker rm "$cid" >/dev/null
echo "status.json: $st"
grep -Eq '^-rw-rw-r-- +999/0 ' <<<"$st" || { echo "/status/status.json must be 0664 999:0"; exit 1; }
echo "smoke test passed"
