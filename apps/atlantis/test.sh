#!/usr/bin/env bash
# Smoke test for a built Atlantis image. Usage: test.sh <image-ref>
# Runs with a read-only rootfs (the posture the chart ships) plus a writable
# data dir + /tmp, then asserts:
#   - `atlantis version` prints the pinned version
#   - `tofu version` works (the bundled OpenTofu execution engine)
#   - the server boots with dummy GitHub creds (no real VCS connectivity needed)
#     and GET /healthz -> 200
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-atlantis-smoke-$$"
BASE="http://127.0.0.1:4141"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "checking atlantis version"
# The entrypoint is `/usr/bin/atlantis server`; override it to reach the
# top-level `version` subcommand.
docker run --rm --entrypoint /usr/bin/atlantis "$IMAGE" version

echo "checking bundled OpenTofu (tofu version)"
docker run --rm --entrypoint /usr/bin/tofu "$IMAGE" version

echo "starting $IMAGE (read-only rootfs; writable data-dir + /tmp; dummy GitHub creds)"
# The entrypoint is `/usr/bin/atlantis server`; we append server flags. Dummy
# creds are enough to boot the server + serve /healthz -- no real VCS needed.
#   --gh-user / --gh-token        dummy GitHub credentials
#   --gh-webhook-secret           dummy webhook HMAC secret
#   --repo-allowlist='*'          allow any repo (gate only)
#   --data-dir=/atlantis-data     writable mount (read-only rootfs)
#   --atlantis-url                required for the server to start
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp \
  --mount type=tmpfs,destination=/atlantis-data \
  -p 127.0.0.1:4141:4141 \
  "$IMAGE" \
  --gh-user=x \
  --gh-token=x \
  --gh-webhook-secret=x \
  --repo-allowlist='*' \
  --data-dir=/atlantis-data \
  --atlantis-url=http://127.0.0.1:4141 >/dev/null

# wait for /healthz to report 200
for i in $(seq 1 30); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/healthz" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    break
  fi
  [ "$i" = 30 ] && { echo "atlantis /healthz did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "/healthz -> 200"

# must run as the nonroot atlantis user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, atlantis + bundled tofu + /healthz OK)"
