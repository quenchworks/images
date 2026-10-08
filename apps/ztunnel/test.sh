#!/usr/bin/env bash
# Smoke test for a built ztunnel image. Usage: test.sh <image-ref> [version]
# `ztunnel version` must report the release, then a real proxy boots standalone the way upstream's
# Development.md runs it (XDS disabled, dedicated mode) and its stats server (:15020/metrics) and
# readiness server (:15021) must answer. Serving mesh traffic needs istiod: the chart's kind gate.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-ztunnel-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT
v="$(docker run --rm "$IMAGE" version 2>&1)"
echo "ztunnel version: $v"
[ -z "$WANT" ] || grep -q "Version:\"$WANT\"" <<<"$v" || { echo "expected Version:\"$WANT\""; exit 1; }
docker run -d --name "$NAME" --cap-add NET_ADMIN -p 127.0.0.1:15020:15020 -p 127.0.0.1:15021:15021 \
  -e XDS_ADDRESS= -e PROXY_MODE=dedicated -e PROXY_WORKLOAD_INFO=default/local/default "$IMAGE" proxy >/dev/null
for i in $(seq 1 30); do
  body="$(curl -sS --max-time 3 http://127.0.0.1:15020/metrics 2>/dev/null || true)"
  grep -q '^# TYPE' <<<"$body" && break
  [ "$i" = 30 ] && { echo "stats server (:15020/metrics) did not answer"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "stats: $(grep -c '^# TYPE' <<<"$body") metric families"
code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 3 http://127.0.0.1:15021/healthz/ready || true)"
echo "readiness :15021/healthz/ready -> $code"
case "$code" in 2??|5??) ;; *) echo "readiness server did not answer"; docker logs "$NAME"; exit 1 ;; esac
docker logs "$NAME" 2>&1 | tail -5
echo "smoke test passed"
