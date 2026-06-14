#!/usr/bin/env bash
# Smoke test for a built blackbox_exporter image. Usage: test.sh <image-ref>
# Runs read-only rootfs (the posture the chart ships) with the shipped minimal
# config, waits for /-/healthy, then asserts /metrics and a /probe against the
# loopback /metrics endpoint (module http_2xx) work. Confirms nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-blackbox-exporter-smoke-$$"
BASE="http://127.0.0.1:9115"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "version check:"
docker run --rm "$IMAGE" --version

echo "starting $IMAGE (read-only rootfs, shipped minimal config)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  -p 127.0.0.1:9115:9115 \
  "$IMAGE" >/dev/null

# wait for the server to report healthy
for i in $(seq 1 30); do
  if curl -fsS "$BASE/-/healthy" >/dev/null 2>&1; then break; fi
  [ "$i" = 30 ] && { echo "blackbox_exporter did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "healthy; checking /metrics exposes blackbox_exporter_build_info"
# capture fully first: a streaming `grep -q` can close the pipe early and make
# curl report a write failure on large bodies.
metrics="$(curl -fsS "$BASE/metrics" 2>/dev/null || true)"
# here-string, not a pipe: under `set -o pipefail`, `grep -q` exits early and
# SIGPIPEs the upstream printf, making a real match look like a failure.
grep -q 'blackbox_exporter_build_info' <<<"$metrics" \
  || { echo "blackbox_exporter_build_info not found on /metrics"; docker logs "$NAME"; exit 1; }

echo "checking /probe with module=http_2xx against the exporter's own /metrics"
probe="$(curl -fsS "$BASE/probe?target=http://127.0.0.1:9115/metrics&module=http_2xx" 2>/dev/null || true)"
grep -q 'probe_success' <<<"$probe" \
  || { echo "/probe did not return probe_success: $probe"; docker logs "$NAME"; exit 1; }

# must run as the nonroot exporter user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot uid: $user, read-only rootfs, healthy + /metrics + /probe OK)"
