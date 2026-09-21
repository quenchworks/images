#!/usr/bin/env bash
# Smoke test for a built OpenCost image. Usage: test.sh <image-ref> <version>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <version>}"
VERSION="${2:?usage: test.sh <image-ref> <version>}"

# must run as the nonroot opencost user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# The pricing JSONs must be where the binary resolves them, path.Join(CONFIG_PATH,
# name) with CONFIG_PATH defaulting to /var/configs. Upstream's own image puts
# them at /models and relies on the chart to override CONFIG_PATH, so a copy of
# that layout would leave this image silently unable to price anything.
# docker cp, not `--entrypoint /bin/sh`: this image has no shell, so any check
# that needs one fails on the harness rather than on the image.
probe="$(mktemp -d)"
cid="$(docker create "$IMAGE")"
for f in default.json aws.json azure.json gcp.json; do
  docker cp "$cid:/var/configs/$f" "$probe/$f" >/dev/null 2>&1 \
    || { echo "missing /var/configs/$f"; docker rm -f "$cid" >/dev/null 2>&1; exit 1; }
  [ -s "$probe/$f" ] || { echo "/var/configs/$f is empty"; docker rm -f "$cid" >/dev/null 2>&1; exit 1; }
done
docker rm -f "$cid" >/dev/null 2>&1 || true

name="opencost-smoke-$$"
docker run -d --rm --name "$name" -p 19003:9003 "$IMAGE" >/dev/null
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# /healthz is served by the agent mux (pkg/cmd/agent), so a 200 proves the
# process got past config load and bound its port. There is no cluster here, so
# the cost queries themselves cannot be exercised.
ok=0
for _ in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:19003/healthz 2>/dev/null || true)"
  if [ "$code" = "200" ]; then ok=1; break; fi
  sleep 1
done
[ "$ok" = "1" ] || { echo "no HTTP 200 on /healthz:9003"; docker logs "$name" 2>&1 | tail -30; exit 1; }

# The ldflag stamp must have reached the binary so the image tag cannot disagree
# with what it runs. Written to a file, never piped into head: under pipefail a
# consumer closing early kills the producer and takes the script with it.
logs="$(mktemp)"
docker logs "$name" >"$logs" 2>&1 || true
grep -q "${VERSION}" "$logs" \
  || { echo "version ${VERSION} not reported at startup"; tail -25 "$logs"; exit 1; }

echo "smoke test passed (nonroot user: $user, /healthz 200 on 9003, version ${VERSION})"
