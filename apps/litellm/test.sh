#!/usr/bin/env bash
# Smoke test for a built litellm image. Usage: test.sh <image-ref> <version>
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> <version>}"
VERSION="${2:?usage: test.sh <image-ref> <version>}"
NAME="quench-litellm-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" -p 127.0.0.1:14000:4000 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 90); do
  if curl -fsS http://127.0.0.1:14000/health/liveliness >/dev/null 2>&1; then ok=1; break; fi
  sleep 1
done
[ "$ok" = 1 ] || { echo "proxy never answered /health/liveliness"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

# the OpenAI-compatible surface is mounted (no models configured, so an empty list)
curl -fsS http://127.0.0.1:14000/v1/models >/dev/null \
  || { echo "/v1/models did not answer"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }

docker exec "$NAME" /opt/litellm/venv/bin/python -c \
  "import importlib.metadata as m; v=m.version('litellm'); print('litellm', v); assert v=='${VERSION}', v"
if docker exec "$NAME" /opt/litellm/venv/bin/python -c "import litellm_enterprise" 2>/dev/null; then
  echo "litellm_enterprise is importable; the image must stay MIT"; exit 1
fi
echo "smoke test passed (nonroot $user, litellm ${VERSION}, no enterprise package)"
