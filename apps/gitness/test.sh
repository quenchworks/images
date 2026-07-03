#!/usr/bin/env bash
# Smoke test for a built Gitness image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "gitness version:"
# The entrypoint is `gitness server`, so override it to query the version.
out="$(docker run --rm --entrypoint /usr/bin/gitness "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default 0.0.0)
echo "$out" | grep -qiE '[1-9][0-9]*\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

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

echo "smoke test passed (nonroot user: $user, embedded UI served HTTP 200 on /)"
