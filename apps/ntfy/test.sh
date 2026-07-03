#!/usr/bin/env bash
# Smoke test for a built ntfy image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# The image entrypoint is `ntfy serve --listen-http :8080`, so override it to
# reach the version subcommand directly.
echo "ntfy version:"
out="$(docker run --rm --entrypoint /usr/bin/ntfy "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "dev")
echo "$out" | grep -qiE 'v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot ntfy user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# start the server (default entrypoint, in-memory cache) and hit the health
# endpoint + the embedded web app.
name="ntfy-smoke-$$"
docker run -d --rm --name "$name" -p 18080:8080 "$IMAGE" >/dev/null
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ok=0
for _ in $(seq 1 30); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/v1/health 2>/dev/null || true)"
  if [ "$code" = "200" ]; then ok=1; break; fi
  sleep 1
done
[ "$ok" = "1" ] || { echo "server did not serve /v1/health with HTTP 200"; docker logs "$name" 2>&1 | tail -20; exit 1; }

# the embedded React web UI must be served at /
app="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/ 2>/dev/null || true)"
[ "$app" = "200" ] || { echo "web app not served at / (got HTTP $app)"; exit 1; }

echo "smoke test passed (nonroot user: $user, /v1/health + web UI HTTP 200)"
