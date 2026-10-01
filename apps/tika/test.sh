#!/usr/bin/env bash
# Smoke test for a built Tika server image. Usage: test.sh <image-ref> [expected-version]
#
# Starts the server and sends it a real document: an HTML page must come back as its
# text from /tika and as metadata (title, content type) from /meta.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
NAME="quench-tika-smoke-$$"
PORT=19998
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT
fail() { echo "FAIL: $*"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" -p "127.0.0.1:$PORT:9998" "$IMAGE" >/dev/null
for i in $(seq 1 90); do
  ver="$(curl -fsS --max-time 2 "http://127.0.0.1:$PORT/version" 2>/dev/null || true)"
  [ -n "$ver" ] && break
  [ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || fail "tika exited"
  sleep 1
done
[ -n "$ver" ] || fail "/version never answered"
echo "$ver"
[ -z "$WANT" ] || grep -q "$WANT" <<<"$ver" || fail "expected version $WANT"

doc='<html><head><title>QuenchWorks smoke</title></head><body><p>extracted by tika</p></body></html>'
txt="$(curl -fsS -X PUT -H 'Content-Type: text/html' -H 'Accept: text/plain' --data-binary "$doc" "http://127.0.0.1:$PORT/tika")"
grep -q 'extracted by tika' <<<"$txt" || fail "/tika returned '$txt'"
meta="$(curl -fsS -X PUT -H 'Content-Type: text/html' -H 'Accept: application/json' --data-binary "$doc" "http://127.0.0.1:$PORT/meta")"
grep -q '"dc:title":"QuenchWorks smoke"' <<<"$meta" || fail "/meta returned '$meta'"
echo "smoke test passed (uid $user, text and metadata extracted)"
