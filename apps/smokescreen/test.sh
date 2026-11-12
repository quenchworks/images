#!/usr/bin/env bash
# Smoke test for a built Smokescreen image. Usage: test.sh <image-ref> [version]
# Two backends (our caddy image) on a private Docker network. Smokescreen allows one by
# --allow-address and must proxy HTTP and CONNECT to it; the other sits on the same private
# range, which Smokescreen refuses by default (its SSRF protection), and must be denied.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="smokescreen-smoke-$$"
BACKEND=ghcr.io/quenchworks/images/caddy:2.11.9
cleanup() { docker rm -f "$TAG" "$TAG-ok" "$TAG-no" >/dev/null 2>&1 || true; docker network rm "$TAG" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$v"
[ -z "$WANT" ] || grep -qF "v$WANT" <<<"$v" || { echo "expected v$WANT"; exit 1; }

docker network create "$TAG" >/dev/null
for n in ok no; do
  docker run -d --name "$TAG-$n" --network "$TAG" --entrypoint /usr/bin/caddy "$BACKEND" respond --listen :8000 "backend-$n" >/dev/null
done
ip() { docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$1"; }
OK="$(ip "$TAG-ok")"; NO="$(ip "$TAG-no")"
docker run -d --name "$TAG" --network "$TAG" --read-only -p 127.0.0.1:14750:4750 "$IMAGE" \
  --listen-port 4750 --allow-address "$OK:8000" >/dev/null

out=""
for _ in $(seq 1 30); do out="$(curl -sS --max-time 5 -x http://127.0.0.1:14750 "http://$OK:8000/" 2>/dev/null || true)"; [ "$out" = backend-ok ] && break; sleep 1; done
[ "$out" = backend-ok ] || { echo "allowed backend not reached over HTTP: $out"; docker logs "$TAG" 2>&1 | tail -15; exit 1; }
echo "HTTP proxy to the allowed backend: $out"
out="$(curl -sS --max-time 5 -p -x http://127.0.0.1:14750 "http://$OK:8000/")"
[ "$out" = backend-ok ] || { echo "allowed backend not reached over CONNECT: $out"; exit 1; }
echo "CONNECT to the allowed backend: $out"
code="$(curl -s --max-time 5 -o "$PWD/.smoke-deny.$$" -w '%{http_code}' -x http://127.0.0.1:14750 "http://$NO:8000/" || true)"
body="$(cat "$PWD/.smoke-deny.$$" 2>/dev/null || true)"; rm -f "$PWD/.smoke-deny.$$"
[ "$code" != 200 ] && ! grep -q backend-no <<<"$body" || { echo "private backend was proxied (HTTP $code)"; exit 1; }
echo "private backend denied (HTTP $code)"
logs="$(docker logs "$TAG" 2>&1)"
grep -qiE 'deny|denied|not allowed|private' <<<"$logs" || { echo "no deny decision logged"; tail -10 <<<"$logs"; exit 1; }
if grep -qE 'panic|read-only file system' <<<"$logs"; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: HTTP and CONNECT to an allowed address, private address denied, user 1001"
