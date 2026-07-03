#!/usr/bin/env bash
# Smoke test for a built Grafana Pyroscope image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-pyroscope-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# 1) version must be stamped from the tag (built via -X, not the default "unknown")
echo "pyroscope version:"
out="$(docker run --rm --entrypoint /usr/bin/pyroscope "$IMAGE" -version 2>&1)"
echo "$out"
echo "$out" | grep -qiE 'pyroscope, version[[:space:]]+[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# 2) server must start and serve /ready on 4040. In the default all-in-one mode
# Pyroscope's readiness gate has cascading "wait N seconds after being ready"
# delays (ingester 15s, then segment writer 30s), so allow up to ~150s.
echo "starting $IMAGE"
docker run -d --name "$NAME" -p 127.0.0.1:4040:4040 "$IMAGE" >/dev/null
for i in $(seq 1 150); do
  if curl -fsS http://127.0.0.1:4040/ready >/dev/null 2>&1; then
    break
  fi
  [ "$i" = 150 ] && { echo "pyroscope did not become ready"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
echo "ready endpoint responded on :4040"

# 3) must run as the nonroot pyroscope user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
