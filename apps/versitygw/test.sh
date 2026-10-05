#!/usr/bin/env bash
# Smoke test for a built versitygw image. Usage: test.sh <image-ref>
# Starts the default posix backend and round-trips an object over signed S3 requests.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-versitygw-smoke-$$"
AK=quenchtest SK=quenchtestsecret0123456789
S3=(curl -fsS --aws-sigv4 "aws:amz:us-east-1:s3" --user "$AK:$SK")

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$NAME" -p 127.0.0.1:7070:7070 \
  -e ROOT_ACCESS_KEY="$AK" -e ROOT_SECRET_KEY="$SK" "$IMAGE" >/dev/null

for i in $(seq 1 30); do
  curl -fsS http://127.0.0.1:7070/health >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { echo "versitygw did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

"${S3[@]}" -X PUT http://127.0.0.1:7070/smoke >/dev/null
"${S3[@]}" -X PUT --data-binary "hello quench" http://127.0.0.1:7070/smoke/hello.txt >/dev/null
got="$("${S3[@]}" http://127.0.0.1:7070/smoke/hello.txt)"
[ "$got" = "hello quench" ] || { echo "object round-trip failed: '$got'"; docker logs "$NAME"; exit 1; }
list="$("${S3[@]}" "http://127.0.0.1:7070/smoke?list-type=2")"
grep -q "<Key>hello.txt</Key>" <<<"$list" || { echo "ListObjectsV2 missing the key"; exit 1; }
# an unsigned request must be refused
code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:7070/smoke/hello.txt)"
[ "$code" = 403 ] || { echo "anonymous GET returned $code, expected 403"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version 2>&1 | awk '/^Version/{print $3}')"
echo "reported version: $ver"
case "$ver" in ""|git|v0.0.0) echo "version not stamped: '$ver'"; exit 1 ;; esac

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (version $ver, user $user)"
