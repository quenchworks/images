#!/usr/bin/env bash
# Smoke test for a built Dependency-Track frontend image. Usage: test.sh <image-ref> [version]
# Serves the UI, its runtime config, and proves the conf.d include hook the chart uses
# for the /api proxy is live.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-dtrack-fe-smoke-$$"
DIR="$(mktemp -d)"
URL=http://127.0.0.1:18081

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$DIR"; }
trap cleanup EXIT

printf 'location /api/ { default_type text/plain; return 200 "conf.d include works\\n"; }\n' > "$DIR/api.conf"
chmod -R a+rX "$DIR"
docker run -d --name "$NAME" -p 127.0.0.1:18081:8080 --tmpfs /tmp --read-only \
  -v "$DIR:/etc/dependency-track-frontend/conf.d:ro" "$IMAGE" >/dev/null
for i in $(seq 1 30); do
  curl -fsS "$URL/" >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { echo "nginx never served"; docker logs "$NAME"; exit 1; }
  sleep 1
done

grep -qi "dependency-track" <<<"$(curl -fsS "$URL/")" || { echo "index.html is not the Dependency-Track UI"; exit 1; }
grep -q '"API_BASE_URL"' <<<"$(curl -fsS "$URL/static/config.json")" || { echo "config.json missing"; exit 1; }
curl -fsSI "$URL/static/config.json" | tr -d '\r' | grep -qi '^cache-control: no-store' || { echo "config.json is cacheable"; exit 1; }
# a client-side route falls back to the SPA
grep -qi "dependency-track" <<<"$(curl -fsS "$URL/projects")" || { echo "SPA fallback broken"; exit 1; }
[ "$(curl -fsS "$URL/api/version")" = "conf.d include works" ] || { echo "conf.d include not applied"; exit 1; }
echo "  UI, config.json (no-store), SPA fallback and the conf.d /api hook all served"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, UI and conf.d hook)"
