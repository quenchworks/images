#!/usr/bin/env bash
# Smoke test for a built Trickster image. Usage: test.sh <image-ref> [version]
# Trickster fronts an HTTP origin (our caddy image, cacheable for 5 minutes) as a reverse proxy
# cache. The first request must come from the origin, the second from Trickster's cache
# (X-Trickster-Result reports the hit), and the metrics port must serve Trickster's metrics.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="trickster-smoke-$$"
cleanup() { docker rm -f "$TAG" "$TAG-origin" >/dev/null 2>&1 || true; docker network rm "$TAG" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" -version 2>&1)"
echo "$v"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

docker network create "$TAG" >/dev/null
docker run -d --name "$TAG-origin" --network "$TAG" --entrypoint /usr/bin/caddy ghcr.io/quenchworks/images/caddy:2.11.9 \
  respond --listen :8000 -H "Cache-Control: max-age=300" -b quench-origin >/dev/null
docker run -d --name "$TAG" --network "$TAG" --read-only -p 127.0.0.1:18480:8480 -p 127.0.0.1:18481:8481 "$IMAGE" \
  -origin-url "http://$TAG-origin:8000" -provider rpc >/dev/null

hdr() { curl -sS --max-time 5 -D - -o "$1" http://127.0.0.1:18480/smoke; }
h1=""
for _ in $(seq 1 30); do h1="$(hdr "$PWD/.tr1.$$" 2>/dev/null || true)"; grep -qi '^x-trickster-result' <<<"$h1" && break; sleep 1; done
b1="$(cat "$PWD/.tr1.$$" 2>/dev/null || true)"; rm -f "$PWD/.tr1.$$"
[ "$b1" = quench-origin ] || { echo "origin body not proxied: $b1"; docker logs "$TAG" 2>&1 | tail -15; exit 1; }
echo "first: $(grep -i '^x-trickster-result' <<<"$h1" | tr -d '\r')"
h2="$(hdr "$PWD/.tr2.$$")"; rm -f "$PWD/.tr2.$$"
r2="$(grep -i '^x-trickster-result' <<<"$h2" | tr -d '\r')"
echo "second: $r2"
grep -qiE 'status=(hit|kmiss|phit)' <<<"$r2" && ! grep -qi 'status=kmiss' <<<"$r2" || { echo "second request was not served from cache"; exit 1; }
m="$(curl -fsS http://127.0.0.1:18481/metrics)"
grep -q '^trickster_' <<<"$m" || { echo "no trickster metrics"; exit 1; }
logs="$(docker logs "$TAG" 2>&1)"
if grep -qE 'panic|read-only file system' <<<"$logs"; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: proxied origin, cache hit on repeat, metrics, user 1001"
