#!/usr/bin/env bash
# Smoke test for a built FileBrowser Quantum image. Usage: test.sh <image-ref> [version]
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
VER="${2:-}"

out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$out"
grep -qE 'v?[0-9]+\.[0-9]+\.[0-9]+' <<<"$out" || { echo "version not stamped"; exit 1; }
[ -z "$VER" ] || grep -q "$VER" <<<"$out" || { echo "expected version $VER"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# the UI is embedded: the server must answer /health and serve the login page from the binary
name="fbq-smoke-$$"
docker run -d --rm --name "$name" -p 18081:8080 -e FILEBROWSER_ADMIN_PASSWORD=smoke-test-pw --tmpfs /tmp "$IMAGE" >/dev/null
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT
ok=0
for _ in $(seq 1 60); do
  h="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18081/health 2>/dev/null || true)"
  r="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18081/ 2>/dev/null || true)"
  if [ "$h" = "200" ] && { [ "$r" = "200" ] || [ "$r" = "302" ]; }; then ok=1; break; fi
  sleep 1
done
[ "$ok" = "1" ] || { echo "server not healthy (health=$h root=$r)"; docker logs "$name" 2>&1 | tail -30; exit 1; }
page="$(curl -sL http://127.0.0.1:18081/)"
grep -qi '<div id="app"\|<script' <<<"$page" || { echo "UI not served"; exit 1; }
echo "smoke test passed (user $user, /health 200, UI served)"
