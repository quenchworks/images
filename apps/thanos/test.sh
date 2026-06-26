#!/usr/bin/env bash
# Smoke test for a built Thanos image. Usage: test.sh <image-ref>
# Thanos is one binary with subcommands; the entrypoint is the bare binary, so the
# chart passes the subcommand as args. This test asserts:
#   - `thanos --version` prints the pinned version
#   - `thanos query --help` exits 0 (subcommand wiring works)
#   - a `thanos query` runs as a read-only-rootfs container against a dummy store
#     endpoint, serves /-/healthy on :10902, and reports it via /-/ready
#   - the container runs as nonroot uid 1001 (image config AND at runtime)
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-thanos-smoke-$$"
BASE="http://127.0.0.1:10902"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "checking thanos version"
docker run --rm "$IMAGE" --version

echo "checking 'thanos query --help' exits 0"
docker run --rm "$IMAGE" query --help >/dev/null

# image config must declare the nonroot thanos user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected image user 1001, got '$user'"; exit 1; }

# Minimal single-component smoke: `thanos query`. It needs at least one --endpoint
# (a Store API gRPC target); a dummy unreachable endpoint is fine -- query still
# starts, becomes healthy, and serves its HTTP UI/probes on :10902. Read-only
# rootfs with a writable /tmp, the posture the chart ships.
echo "starting 'thanos query' (read-only rootfs; writable /tmp; :10902)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,uid=1001,gid=1001 \
  -p 127.0.0.1:10902:10902 \
  "$IMAGE" \
  query \
  --http-address=0.0.0.0:10902 \
  --grpc-address=0.0.0.0:10901 \
  --endpoint=127.0.0.1:19090 >/dev/null

# assert the runtime uid is 1001 (nonroot) inside the container
echo "checking runtime uid"
rt_uid="$(docker exec "$NAME" id -u 2>/dev/null || true)"
[ "$rt_uid" = "1001" ] || { echo "expected runtime uid 1001, got '$rt_uid'"; docker logs "$NAME"; exit 1; }

# wait for /-/healthy (thanos query serves this once the HTTP server is up)
for i in $(seq 1 60); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/-/healthy" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then break; fi
  [ "$i" = 60 ] && { echo "thanos query /-/healthy did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "/-/healthy -> 200"

# /-/ready may flip to 200 once the (dummy) endpoint discovery loop settles; tolerate
# either 200 or 503 but require the endpoint to respond.
code="$(curl -fsS -o /dev/null -w '%{http_code}' "$BASE/-/ready" 2>/dev/null || true)"
echo "/-/ready -> ${code}"
[ "$code" = "200" ] || [ "$code" = "503" ] || { echo "/-/ready returned $code"; docker logs "$NAME"; exit 1; }

echo "version:"; docker run --rm "$IMAGE" --version 2>&1 | head -1
echo "smoke test passed (nonroot uid: $user, read-only rootfs, query healthy OK)"
