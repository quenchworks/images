#!/usr/bin/env bash
# Smoke test for a built tusd image. Usage: test.sh <image-ref> [version]
# A real resumable upload over the tus protocol: create an upload, send it in two PATCH
# chunks (the second resumes at the offset HEAD reports), then download it and compare.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="tusd-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" -version 2>&1)"
echo "$v" | head -2
[ -z "$WANT" ] || grep -qF "v$WANT" <<<"$v" || { echo "expected v$WANT"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /data:uid=1001,gid=1001 -p 127.0.0.1:18080:8080 "$IMAGE" \
  -upload-dir /data -port 8080 >/dev/null
B=http://127.0.0.1:18080/files/
for _ in $(seq 1 30); do curl -fsS -o /dev/null -X OPTIONS "$B" 2>/dev/null && break; sleep 1; done

head -c 300000 /dev/urandom > "$WORK/blob"
head -c 120000 "$WORK/blob" > "$WORK/part1"; tail -c +120001 "$WORK/blob" > "$WORK/part2"
loc="$(curl -fsS -D - -o /dev/null -X POST "$B" -H 'Tus-Resumable: 1.0.0' -H 'Upload-Length: 300000' \
  | tr -d '\r' | awk 'tolower($1)=="location:"{print $2}')"
[ -n "$loc" ] || { echo "no upload created"; docker logs "$NAME" 2>&1 | tail -10; exit 1; }
case "$loc" in http*) ;; *) loc="http://127.0.0.1:18080$loc";; esac
echo "created $loc"
curl -fsS -o /dev/null -X PATCH "$loc" -H 'Tus-Resumable: 1.0.0' -H 'Upload-Offset: 0' \
  -H 'Content-Type: application/offset+octet-stream' --data-binary @"$WORK/part1"
off="$(curl -fsS -I "$loc" -H 'Tus-Resumable: 1.0.0' | tr -d '\r' | awk 'tolower($1)=="upload-offset:"{print $2}')"
[ "$off" = 120000 ] || { echo "offset after the first chunk: $off"; exit 1; }
curl -fsS -o /dev/null -X PATCH "$loc" -H 'Tus-Resumable: 1.0.0' -H "Upload-Offset: $off" \
  -H 'Content-Type: application/offset+octet-stream' --data-binary @"$WORK/part2"
curl -fsS -o "$WORK/back" "$loc"
cmp -s "$WORK/blob" "$WORK/back" || { echo "downloaded file differs from the upload"; exit 1; }
echo "resumed at $off, downloaded 300000 bytes intact"
if docker logs "$NAME" 2>&1 | grep -qE 'panic|read-only file system'; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: create, two-chunk resumable upload, download, user 1001"
