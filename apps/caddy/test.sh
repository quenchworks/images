#!/usr/bin/env bash
# Smoke test for a built Caddy image. Usage: test.sh <image-ref>
# Runs with a read-only rootfs (the posture the chart ships) with /data, /config
# and /tmp writable, a minimal Caddyfile that responds 200 on :8080, then asserts:
#   - `caddy version` prints the pinned version
#   - GET :8080/         -> 200 (responds "ok 200")
#   - GET :2019/config/  -> 200 (admin API up)
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-caddy-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoke-XXXXXX")"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

echo "checking caddy version"
# The image entrypoint is `caddy run --config ...`; override it to read version.
docker run --rm --entrypoint /usr/bin/caddy "$IMAGE" version

# Minimal Caddyfile: admin API on all interfaces (so we can probe from host),
# a site on :8080 that responds 200.
cat > "$WORK/Caddyfile" <<'EOF'
{
	admin :2019
}
:8080 {
	respond "ok 200" 200
}
EOF

echo "starting $IMAGE (read-only rootfs; writable /data /config /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp \
  --tmpfs /data \
  --tmpfs /config \
  -v "$WORK/Caddyfile:/etc/caddy/Caddyfile:ro" \
  -p 127.0.0.1:8080:8080 \
  -p 127.0.0.1:2019:2019 \
  "$IMAGE" >/dev/null

# wait for :8080 to respond
for i in $(seq 1 30); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "http://127.0.0.1:8080/" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    break
  fi
  [ "$i" = 30 ] && { echo "caddy :8080 did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo ":8080/ -> 200"

code="$(curl -fsS -o /dev/null -w '%{http_code}' "http://127.0.0.1:2019/config/" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "admin :2019/config/ returned $code, expected 200"; docker logs "$NAME"; exit 1; }
echo ":2019/config/ -> 200"

# must run as the nonroot caddy user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, :8080 + admin OK)"
