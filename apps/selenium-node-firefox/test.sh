#!/usr/bin/env bash
# Smoke test for a built selenium-node-firefox image. Usage: test.sh <image-ref> [version]
# Real browser work: the image runs as a standalone grid on a read-only root, /status
# must report ready, then a WebDriver session is created through the grid, Firefox
# loads a data: URL, the page title is read back, and the session is deleted.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="selenium-firefox-smoke-$$"
URL=http://127.0.0.1:14445
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT
J='Content-Type: application/json; charset=utf-8'

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp --tmpfs /home/nonroot:uid=1001,gid=1001 --shm-size 256m \
  -p 127.0.0.1:14445:4444 "$IMAGE" standalone --config /etc/selenium/config.toml >/dev/null
status=""
for _ in $(seq 1 90); do
  status="$(curl -fsS "$URL/status" 2>/dev/null || true)"
  grep -q '"ready": *true' <<<"$status" && break
  sleep 1
done
grep -q '"ready": *true' <<<"$status" || { echo "grid never ready: $status" | head -c 800; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
if [ -n "$WANT" ]; then
  logs="$(docker logs "$NAME" 2>&1)"
  grep -q "Started Selenium Standalone $WANT" <<<"$logs" || { echo "standalone did not report version $WANT"; echo "$logs" | tail -20; exit 1; }
fi

new="$(curl -sS -H "$J" -d '{"capabilities":{"alwaysMatch":{"browserName":"firefox"}}}' "$URL/session")"
sid="$(sed -n 's/.*"sessionId": *"\([^"]*\)".*/\1/p' <<<"$new" | head -1)"
[ -n "$sid" ] || { echo "no session created: $new" | head -c 800; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
browser="$(sed -n 's/.*"browserVersion": *"\([^"]*\)".*/\1/p' <<<"$new" | head -1)"

curl -fsS -H "$J" -d '{"url":"data:text/html,<title>quench-selenium</title><h1>ok</h1>"}' "$URL/session/$sid/url" >/dev/null
title="$(curl -fsS "$URL/session/$sid/title")"
grep -q '"value": *"quench-selenium"' <<<"$title" || { echo "unexpected title: $title"; exit 1; }
curl -fsS -X DELETE "$URL/session/$sid" >/dev/null

echo "smoke test passed (selenium ${WANT:-?} standalone, firefox $browser, uid $user, title read back)"
