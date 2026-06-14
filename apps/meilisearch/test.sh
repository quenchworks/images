#!/usr/bin/env bash
# Smoke test for a built Meilisearch image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the data dir + /tmp (the read-only
# posture the chart ships), in MEILI_ENV=production with a master key. Waits for the
# unauthenticated GET /health to return 200, then PROVES the auth model end-to-end:
# GET /keys WITH the master key -> 200, WITHOUT the key -> 401/403. Finally asserts
# nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-meilisearch-smoke-$$"
KEY="testkey-smoke-0123456789abcdef"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /meili_data + /tmp, MEILI_ENV=production)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /meili_data:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -e MEILI_MASTER_KEY="$KEY" \
  -e MEILI_ENV=production \
  -p 127.0.0.1:7700:7700 \
  "$IMAGE" >/dev/null

# wait for the unauthenticated /health (200 once the HTTP server is up).
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:7700/health" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    echo "/health OK after ${i}s"
    break
  fi
  if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
    echo "container exited"; docker logs "$NAME"; exit 1
  fi
  [ "$i" = 60 ] && { echo "meilisearch /health did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# /health returns the expected JSON payload
curl -fsS "http://127.0.0.1:7700/health" | grep -q '"available"' \
  || { echo "/health payload unexpected"; docker logs "$NAME"; exit 1; }

# DASHBOARD PROOF: GET / serves the bundled mini-dashboard HTML (the dashboard page,
# which then prompts for the master key). Confirm the dashboard is embedded -- not a
# 404 / "dashboard not available" stub. With mini-dashboard compiled in, / returns the
# HTML shell that loads the dashboard JS bundle.
echo "dashboard check: GET / serves the embedded mini-dashboard HTML"
root_code="$(curl -s -o /tmp/meili-root.html -w '%{http_code}' "http://127.0.0.1:7700/" 2>/dev/null || true)"
[ "$root_code" = "200" ] || { echo "GET / expected 200, got $root_code"; docker logs "$NAME"; exit 1; }
grep -qi "Mini-dashboard | Meilisearch\|<title>Mini-dashboard" /tmp/meili-root.html \
  || { echo "GET / did not return the bundled mini-dashboard HTML"; head -c 400 /tmp/meili-root.html; docker logs "$NAME"; exit 1; }
echo "mini-dashboard is bundled and served at / (HTML title present)"

# AUTH PROOF: GET /keys WITH the master key -> 200
echo "auth check: GET /keys WITH master key (expect 200)"
code="$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer ${KEY}" "http://127.0.0.1:7700/keys" 2>/dev/null || true)"
[ "$code" = "200" ] || { echo "GET /keys with key expected 200, got $code"; docker logs "$NAME"; exit 1; }

# AUTH PROOF: GET /keys WITHOUT the key -> 401/403
echo "auth check: GET /keys WITHOUT master key (expect 401/403)"
code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:7700/keys" 2>/dev/null || true)"
case "$code" in
  401|403) echo "unauthenticated correctly rejected ($code)" ;;
  *) echo "GET /keys without key expected 401/403, got $code"; docker logs "$NAME"; exit 1 ;;
esac

# AUTH PROOF: wrong key -> 401/403
echo "auth check: GET /keys with WRONG key (expect 401/403)"
code="$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer wrong-key" "http://127.0.0.1:7700/keys" 2>/dev/null || true)"
case "$code" in
  401|403) echo "wrong key correctly rejected ($code)" ;;
  *) echo "GET /keys with wrong key expected 401/403, got $code"; docker logs "$NAME"; exit 1 ;;
esac

# must run as the nonroot meilisearch user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/meilisearch --version
echo "smoke test passed (nonroot user: $user, read-only rootfs, /health 200, auth with/without/wrong key proven)"
