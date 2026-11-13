#!/usr/bin/env bash
# Smoke test for a built Owncast image. Usage: test.sh <image-ref> [version]
# Owncast runs on a read-only root with /app/data on a volume. The image's own ffmpeg pushes a
# test pattern over RTMP with the stream key; Owncast must go online and serve an HLS playlist with
# segments (which proves its ffmpeg transcode). The admin API takes the admin password and refuses
# a wrong one.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-owncast-smoke-$$"
NET="$TAG-net"
PW="admin-$$-pass"
SK="key-$$-stream"
URL=http://127.0.0.1:18080
cleanup() { docker rm -f "$TAG" "$TAG-src" >/dev/null 2>&1 || true; docker volume rm "$TAG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
docker network create "$NET" >/dev/null
docker volume create "$TAG" >/dev/null
docker run -d --name "$TAG" --network "$NET" --network-alias owncast --read-only --tmpfs /tmp:uid=1001,gid=1001 \
  -v "$TAG:/app/data" -p 127.0.0.1:18080:8080 "$IMAGE" -adminpassword "$PW" -streamkey "$SK" >/dev/null
st=""
for i in $(seq 1 60); do st="$(curl -fsS "$URL/api/status" 2>/dev/null || true)"; [ -n "$st" ] && break
  [ "$i" = 60 ] && { echo "/api/status never answered"; docker logs "$TAG" 2>&1 | tail -30; exit 1; }; sleep 1; done
ver="$(jq -er '.versionNumber' <<<"$st")"
echo "owncast $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

code="$(curl -s -o /dev/null -w '%{http_code}' -u admin:wrong "$URL/api/admin/status")"
[ "$code" = 401 ] || { echo "a wrong admin password got HTTP $code"; exit 1; }
curl -fsS -u "admin:$PW" "$URL/api/admin/status" >/dev/null
echo "  admin API: password accepted, wrong one refused"

docker run -d --name "$TAG-src" --network "$NET" --entrypoint /usr/bin/ffmpeg "$IMAGE" -hide_banner -loglevel error -re \
  -f lavfi -i testsrc=size=640x360:rate=25 -f lavfi -i sine=frequency=440 -t 120 \
  -c:v libx264 -preset veryfast -g 50 -c:a aac -f flv "rtmp://owncast:1935/live/$SK" >/dev/null
online=""
for _ in $(seq 1 60); do
  online="$(curl -fsS "$URL/api/status" | jq -er '.online' 2>/dev/null || true)"
  [ "$online" = true ] && break; sleep 1
done
[ "$online" = true ] || { echo "stream never went online"; docker logs "$TAG" 2>&1 | tail -30; docker logs "$TAG-src" 2>&1 | tail -10; exit 1; }
echo "  RTMP stream online"
pl=""
for _ in $(seq 1 60); do
  pl="$(curl -fsS "$URL/hls/stream.m3u8" 2>/dev/null || true)"
  if grep -q '^#EXTM3U' <<<"$pl"; then
    v="$(grep -m1 '\.m3u8' <<<"$pl" || true)"
    [ -n "$v" ] && grep -q '\.ts\|\.m4s' <<<"$(curl -fsS "$URL/hls/$v" 2>/dev/null || true)" && break
  fi
  sleep 1
done
grep -q '\.ts\|\.m4s' <<<"$(curl -fsS "$URL/hls/$v" 2>/dev/null || true)" || { echo "no HLS segments: $pl"; docker logs "$TAG" 2>&1 | tail -30; exit 1; }
echo "  HLS playlist with segments: /hls/$v"
if docker logs "$TAG" 2>&1 | grep -qiE 'read-only file system|panic'; then echo "crashed or wrote to the root"; docker logs "$TAG" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (owncast $ver: admin auth, RTMP ingest with the stream key, ffmpeg transcode to HLS, read-only root; user $user)"
