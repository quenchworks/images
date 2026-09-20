#!/usr/bin/env bash
# Smoke test for a built Gitness image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# must run as the nonroot gitness user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# start the server on a temp host port and curl the embedded UI (HTTP 200 on /),
# which proves the go:embed'd web frontend was actually built into the binary.
name="gitness-smoke-$$"
docker run -d --rm --name "$name" -p 13000:3000 "$IMAGE" >/dev/null
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ok=0
for _ in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:13000/ 2>/dev/null || true)"
  if [ "$code" = "200" ]; then ok=1; break; fi
  sleep 1
done
[ "$ok" = "1" ] || { echo "server did not serve / with HTTP 200"; docker logs "$name" 2>&1 | tail -30; exit 1; }

# The ldflag stamp must have reached the binary. gitness has NO --version flag (3.3.0
# offers only help/migrate/server), so this used to call one that does not exist and
# failed every run. The running server reports it instead, which also proves the API
# is up rather than only the static UI.
ver="$(curl -s http://127.0.0.1:13000/api/v1/system/version 2>/dev/null | tr -d '"[:space:]')"
echo "gitness version: $ver"
echo "$ver" | grep -qE '^[1-9][0-9]*\.[0-9]+\.[0-9]+$' \
  || { echo "version not stamped (got '$ver')"; exit 1; }

echo "smoke test passed (nonroot user: $user, embedded UI served HTTP 200 on /, version $ver)"
