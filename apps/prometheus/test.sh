#!/usr/bin/env bash
# Smoke test for a built Prometheus image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the TSDB + /tmp (the read-only posture
# the chart ships). Waits for /-/healthy, then asserts /-/ready, the self-scrape
# `up` metric via /api/v1/query, that the embedded UI is served at /, and that
# promtool works. Confirms the container runs as nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-prometheus-smoke-$$"
BASE="http://127.0.0.1:9090"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /prometheus + /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /prometheus:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -p 127.0.0.1:9090:9090 \
  "$IMAGE" >/dev/null

# wait for the server to report healthy
for i in $(seq 1 30); do
  if curl -fsS "$BASE/-/healthy" >/dev/null 2>&1; then
    break
  fi
  [ "$i" = 30 ] && { echo "prometheus did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "healthy; checking /-/healthy body"
healthy="$(curl -fsS "$BASE/-/healthy")"
echo "$healthy" | grep -qi "Healthy" || { echo "unexpected /-/healthy body: $healthy"; exit 1; }

echo "checking /-/ready"
for i in $(seq 1 30); do
  if curl -fsS "$BASE/-/ready" >/dev/null 2>&1; then break; fi
  [ "$i" = 30 ] && { echo "prometheus did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "querying the self-scrape 'up' metric (waiting for the first scrape)"
got=""
for i in $(seq 1 30); do
  resp="$(curl -fsS "$BASE/api/v1/query?query=up" 2>/dev/null || true)"
  # JSON: {"status":"success","data":{"resultType":"vector","result":[{...,"value":[ts,"1"]}]}}
  if printf '%s' "$resp" | grep -q '"status":"success"' \
     && printf '%s' "$resp" | grep -q '"__name__":"up"'; then
    got="ok"; break
  fi
  sleep 1
done
[ "$got" = "ok" ] || { echo "self-scrape 'up' metric not returned; last resp: ${resp:-<none>}"; docker logs "$NAME"; exit 1; }

echo "checking the embedded UI is served at /"
# / redirects to the SPA; follow redirects and assert we get HTML back.
ui="$(curl -fsSL "$BASE/" 2>/dev/null || true)"
printf '%s' "$ui" | grep -qi "<!doctype html\|<html" \
  || { echo "embedded UI did not return HTML at /"; docker logs "$NAME"; exit 1; }

echo "checking promtool"
docker exec "$NAME" /usr/bin/promtool --version

# must run as the nonroot prometheus user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/prometheus --version 2>&1 | head -1
echo "smoke test passed (nonroot uid: $user, read-only rootfs, healthy+ready+up+UI OK)"
