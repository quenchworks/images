#!/usr/bin/env bash
# Smoke test for a built Apache Traffic Server image. Usage: test.sh <image-ref> [version]
#
# An open port proves nothing, so this test needs answers only a working ATS gives:
#   1. traffic_server --version reports the version.
#   2. The image boots as uid 1001 on a READ-ONLY rootfs with tmpfs on the three state dirs,
#      the way the chart runs it, and answers an unmapped request with its own 404
#      (Server: ATS/<version>).
#   3. Reverse proxy and cache: an origin ATS serves a file through the statichit plugin, a
#      front ATS maps / to it. The body must come through the front, and the second request
#      must be a fresh cache hit (proxy.process.http.cache_hit_fresh), which needs the
#      storage.config path and the cache dir to be right.
# Configs go in with `docker cp` (a bind mount of /tmp is refused under Docker Desktop, and
# docker cp is refused on a read-only rootfs), so step 3 runs on a writable rootfs.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
TAG="quench-ats-smoke-$$"
NET="$TAG-net"
WORK="$(mktemp -d)"
free_port() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }

cleanup() {
  docker rm -f "$TAG-ro" "$TAG-origin" "$TAG-front" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

dump() { for c in "$TAG-ro" "$TAG-origin" "$TAG-front"; do echo "--- $c"; docker logs "$c" 2>&1 | tail -20 || true; done; }

ver_out="$(docker run --rm --entrypoint /usr/bin/traffic_server "$IMAGE" --version 2>&1)"
echo "$ver_out"
grep -q 'Apache Traffic Server - traffic_server - ' <<<"$ver_out" || { echo "no version banner"; exit 1; }
if [ -n "${2:-}" ]; then
  grep -qF "traffic_server - ${2} -" <<<"$ver_out" || { echo "expected version $2"; exit 1; }
fi

# 2. read-only boot
PORT="$(free_port)"
docker run -d --name "$TAG-ro" --read-only --tmpfs /tmp \
  --tmpfs /var/log/trafficserver:uid=1001 --tmpfs /run/trafficserver:uid=1001 \
  --tmpfs /var/cache/trafficserver:uid=1001 \
  -p "127.0.0.1:${PORT}:8080" "$IMAGE" >/dev/null
hdr=""
for i in $(seq 1 30); do
  hdr="$(curl -sS -m 5 -D - -o /dev/null "http://127.0.0.1:${PORT}/" 2>/dev/null || true)"
  grep -qi '^Server: ATS/' <<<"$hdr" && break
  [ "$i" = 30 ] && { echo "ATS never answered on a read-only rootfs"; dump; exit 1; }
  sleep 1
done
printf '%s\n' "$hdr" | sed -n '1,4p'
grep -q '^HTTP/1.1 404' <<<"$hdr" || { echo "expected 404 for an unmapped request"; exit 1; }
if [ -n "${2:-}" ]; then
  grep -qi "^Server: ATS/${2}" <<<"$hdr" || { echo "expected Server: ATS/$2"; exit 1; }
fi
logs="$(docker logs "$TAG-ro" 2>&1)"
if grep -qiE 'read-only file system|permission denied|FATAL|ALERT|EMERGENCY' <<<"$logs"; then
  echo "errors on a read-only rootfs:"; echo "$logs"; exit 1
fi
echo "read-only boot ok"

# 3. reverse proxy + cache
docker network create "$NET" >/dev/null
echo "quench-ats-smoke $$" > "$WORK/hello.txt"
echo 'map / http://127.0.0.1/ @plugin=statichit.so @pparam=--file-path=/etc/trafficserver/hello.txt @pparam=--mime-type=text/plain @pparam=--max-age=300' > "$WORK/remap.config"
docker create --name "$TAG-origin" --network "$NET" "$IMAGE" >/dev/null
docker cp "$WORK/remap.config" "$TAG-origin:/etc/trafficserver/remap.config"
docker cp "$WORK/hello.txt" "$TAG-origin:/etc/trafficserver/hello.txt"
docker start "$TAG-origin" >/dev/null

echo "map / http://$TAG-origin:8080/" > "$WORK/remap.config"
PORT="$(free_port)"
docker create --name "$TAG-front" --network "$NET" -p "127.0.0.1:${PORT}:8080" "$IMAGE" >/dev/null
docker cp "$WORK/remap.config" "$TAG-front:/etc/trafficserver/remap.config"
docker start "$TAG-front" >/dev/null

body=""
for i in $(seq 1 30); do
  body="$(curl -sS -m 5 "http://127.0.0.1:${PORT}/" 2>/dev/null || true)"
  grep -qF "quench-ats-smoke $$" <<<"$body" && break
  [ "$i" = 30 ] && { echo "the front never returned the origin's body: $body"; dump; exit 1; }
  sleep 1
done
echo "proxied: $body"
curl -sS -m 5 -o /dev/null "http://127.0.0.1:${PORT}/"
hits="$(docker exec "$TAG-front" /usr/bin/traffic_ctl metric get proxy.process.http.cache_hit_fresh | awk '{print $2}')"
[ "${hits:-0}" -ge 1 ] || { echo "no fresh cache hit (cache_hit_fresh=$hits)"; dump; exit 1; }
echo "cache hit ok (cache_hit_fresh=$hits)"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only boot, reverse proxy + cache)"
