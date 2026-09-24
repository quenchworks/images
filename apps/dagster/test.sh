#!/usr/bin/env bash
# Smoke test for a built dagster image. Usage: test.sh <image-ref> <version>
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> <version>}"
VERSION="${2:?usage: test.sh <image-ref> <version>}"
NAME="quench-dagster-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# default command: the webserver with an empty workspace
docker run -d --name "$NAME" --read-only --tmpfs /tmp --tmpfs /var/lib/dagster:uid=1001,gid=1001 \
  -p 127.0.0.1:13000:3000 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 90); do
  info="$(curl -fsS http://127.0.0.1:13000/server_info 2>/dev/null || true)"
  [ -n "$info" ] && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "webserver never answered /server_info"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
echo "$info"
echo "$info" | grep -q "\"dagster_version\": *\"${VERSION}\"" || { echo "version ${VERSION} not reported"; exit 1; }

# GraphQL answers (the UI's API)
curl -fsS -H 'Content-Type: application/json' -d '{"query":"{ version }"}' http://127.0.0.1:13000/graphql | grep -q "${VERSION}" \
  || { echo "graphql did not report ${VERSION}"; exit 1; }

# the daemon role starts from the same image
docker run --rm --entrypoint /opt/dagster/venv/bin/dagster-daemon "$IMAGE" --help >/dev/null
echo "smoke test passed (nonroot $user, dagster ${VERSION}, read-only root)"
