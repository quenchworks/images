#!/usr/bin/env bash
# Smoke test for a built OPA image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-opa-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# default command runs the policy server on :8181
docker run -d --name "$NAME" -p 127.0.0.1:8181:8181 "$IMAGE" >/dev/null

# /health returns 200 once the server is up
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8181/health >/dev/null 2>&1; then
    break
  fi
  [ "$i" = 30 ] && { echo "opa server did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "healthy; evaluating a trivial policy via the Data API"
# POST a query to the default document; an empty policy set returns {} (HTTP 200)
curl -fsS http://127.0.0.1:8181/v1/data >/dev/null \
  || { echo "data API did not respond"; docker logs "$NAME"; exit 1; }

# version must be stamped (built from the tag, not "0.0.0")
# --entrypoint is required. The image's entrypoint is
#   /usr/bin/opa run --server --addr=0.0.0.0:8181
# so a bare `docker run "$IMAGE" version` appends "version" to THAT, where opa reads it as
# a bundle path and dies with
#   error: load error: stat version: no such file or directory
# The bare form therefore never worked; it only surfaced when this workflow started running
# test.sh at all.
ver="$(docker run --rm --entrypoint /usr/bin/opa "$IMAGE" version | awk '/^Version:/{print $2}')"
echo "reported version: $ver"
case "$ver" in
  ""|0.0.0|*dev*) echo "version not stamped: '$ver'"; exit 1 ;;
esac

# must run as the nonroot opa user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user)"
