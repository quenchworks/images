#!/usr/bin/env bash
# Smoke test for a built Vector image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the data dir and /tmp (the read-only
# posture the chart ships), mounts a minimal config (demo_logs -> console + api on
# :8686), waits for the API /health to return 200, then asserts nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-vector-smoke-$$"
CFGDIR="$(mktemp -d)"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  rm -rf "$CFGDIR"
}
trap cleanup EXIT

cat > "$CFGDIR/vector.yaml" <<'CFG'
api:
  enabled: true
  address: "0.0.0.0:8686"
sources:
  demo:
    type: demo_logs
    format: json
    interval: 1
sinks:
  out:
    type: console
    inputs: ["demo"]
    encoding:
      codec: json
CFG
chmod 0644 "$CFGDIR/vector.yaml"

echo "starting $IMAGE (read-only rootfs, tmpfs /vector-data-dir + /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /vector-data-dir:rw,mode=1700 \
  --tmpfs /tmp:rw,mode=1777 \
  -v "$CFGDIR/vector.yaml:/etc/vector/vector.yaml:ro" \
  -p 127.0.0.1:8686:8686 \
  "$IMAGE" >/dev/null

# wait for the API /health (200 once Vector's API server is up).
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:8686/health" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    echo "/health OK after ${i}s"
    break
  fi
  if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
    echo "container exited"; docker logs "$NAME"; exit 1
  fi
  [ "$i" = 60 ] && { echo "vector /health did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# /health returns the expected JSON payload
curl -fsS "http://127.0.0.1:8686/health" | grep -q '"ok"' \
  || { echo "/health payload unexpected"; docker logs "$NAME"; exit 1; }

# must run as the nonroot vector user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/vector --version
echo "smoke test passed (nonroot user: $user, read-only rootfs, api /health 200)"
