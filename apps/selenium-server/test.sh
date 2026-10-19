#!/usr/bin/env bash
# Smoke test for a built selenium-server image. Usage: test.sh <image-ref> [version]
# Starts the hub on a read-only root, checks /status reports the expected version,
# asks the GraphQL endpoint for the grid, and sends a real POST /session: with no node
# registered the request must sit in the queue and come back "session not created"
# after the request timeout, which proves router, queue and distributor are wired.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="selenium-server-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp --tmpfs /home/nonroot:uid=1001,gid=1001 \
  -p 127.0.0.1:14444:4444 "$IMAGE" hub --session-request-timeout 5 >/dev/null
status=""
for _ in $(seq 1 90); do
  status="$(curl -fsS http://127.0.0.1:14444/status 2>/dev/null || true)"
  [ -n "$status" ] && break
  sleep 1
done
[ -n "$status" ] || { echo "hub never answered /status"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
grep -q '"message"' <<<"$status" || { echo "unexpected /status: $status"; exit 1; }

logs="$(docker logs "$NAME" 2>&1)"
if [ -n "$WANT" ]; then
  grep -q "Started Selenium Hub $WANT" <<<"$logs" || { echo "hub did not report version $WANT"; echo "$logs" | tail -20; exit 1; }
fi

gql="$(curl -fsS -H 'Content-Type: application/json' -d '{"query":"{ grid { totalSlots sessionCount version } }"}' http://127.0.0.1:14444/graphql)"
grep -q '"totalSlots"' <<<"$gql" || { echo "graphql did not answer: $gql"; exit 1; }

code="$(curl -sS -o /tmp/sel-smoke-$$.json -w '%{http_code}' -H 'Content-Type: application/json; charset=utf-8' \
  -d '{"capabilities":{"alwaysMatch":{"browserName":"firefox"}}}' http://127.0.0.1:14444/session || true)"
body="$(cat /tmp/sel-smoke-$$.json 2>/dev/null || true)"; rm -f /tmp/sel-smoke-$$.json
[ "$code" = 500 ] && grep -q 'session not created' <<<"$body" || { echo "POST /session: expected 500 session not created, got $code: $body" | head -c 600; exit 1; }

echo "smoke test passed (selenium hub ${WANT:-?}, uid $user, queue answered a session request)"
