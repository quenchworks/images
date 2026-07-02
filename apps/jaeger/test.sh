#!/usr/bin/env bash
# Smoke test for a built Jaeger v2 image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-jaeger-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "jaeger version:"
out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "dev")
echo "$out" | grep -qE '"gitVersion":"v[0-9]+\.[0-9]+\.[0-9]+"' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot jaeger user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "starting all-in-one (embedded config, in-memory storage)"
docker run -d --name "$NAME" -p 127.0.0.1:16686:16686 "$IMAGE" >/dev/null

# the query UI serves its React app (embedded assets) on 16686
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:16686/ 2>/dev/null | grep -qi 'Jaeger UI'; then
    break
  fi
  [ "$i" = 30 ] && { echo "jaeger UI did not come up"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "smoke test passed (nonroot user: $user, UI serving embedded assets on 16686)"
