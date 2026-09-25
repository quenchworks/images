#!/usr/bin/env bash
# Smoke test for a built hubble-ui image. Usage: test.sh <image-ref> [version]
# nginx on a read-only root must serve the SPA, its hashed bundle, deep links through
# the index fallback and /healthz, and answer /api/ with a gateway error (no backend in
# this test, which proves the proxy is wired). The chart gate runs it with the backend.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="hubble-ui-smoke-$$"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
docker run -d --name "$NAME" --read-only --tmpfs /tmp -p 127.0.0.1:18081:8081 "$IMAGE" >/dev/null
B=http://127.0.0.1:18081
ok=0
for _ in $(seq 1 30); do curl -fsS "$B/healthz" >/dev/null 2>&1 && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] || { echo "nginx never answered"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
idx="$(curl -fsS "$B/")"
echo "$idx" | grep -q '<div id="app"' || echo "$idx" | grep -q 'bundle.main' || { echo "index is not the Hubble UI"; exit 1; }
js="$(echo "$idx" | grep -oE 'bundle\.main\.[0-9a-f]+\.js' | head -1)"
[ -n "$js" ] && curl -fsS -o /dev/null "$B/$js" || { echo "main bundle not served"; exit 1; }
curl -fsS "$B/kube-system/some/deep/link" | grep -q 'bundle.main' || { echo "no SPA fallback"; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' "$B/api/")"
[ "$code" = 502 ] || { echo "/api/ gave $code, want 502 without a backend"; exit 1; }
echo "smoke test passed (hubble-ui ${2:-?}, uid $user, SPA, bundle, fallback, api proxy)"
