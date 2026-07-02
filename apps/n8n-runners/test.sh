#!/usr/bin/env bash
# Smoke test for a built n8n-runners image. Usage: test.sh <image-ref>
#
# The image has NO shell (minimal apko), so every check overrides the entrypoint with an
# exec-form command. We verify: the launcher binary runs + reads its config, the JS runtime
# and Python venv work, the Python runner deps import, and all operator package managers are
# present. A full end-to-end run needs a live n8n task broker, which a hermetic smoke can't
# provide, so we assert the launcher fails CLEANLY on the missing broker auth token (which
# proves the binary loads /etc/n8n-task-runners.json and starts up).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "== launcher boots + reads config (expected: complains about missing auth token) =="
out="$(docker run --rm "$IMAGE" 2>&1 || true)"
echo "$out" | grep -qi 'N8N_RUNNERS_AUTH_TOKEN' \
  || { echo "launcher did not reach config load; output was:"; echo "$out" | tail -20; exit 1; }
echo "  ok: launcher runs and loads config"

echo "== node runtime (JS runner) =="
docker run --rm --entrypoint /usr/bin/node "$IMAGE" --version

echo "== python venv imports the runner's pinned deps =="
docker run --rm --entrypoint /opt/runners/task-runner-python/.venv/bin/python "$IMAGE" \
  -c "import urllib3, websockets; print('py-runner deps ok:', urllib3.__version__)"

echo "== JS runner entry present =="
docker run --rm --entrypoint /usr/bin/node "$IMAGE" \
  -e "require('fs').accessSync('/opt/runners/task-runner-javascript/dist/start.js'); console.log('js start.js ok')"

echo "== operator package managers present =="
for pm in "uv:--version" "poetry:--version" "npm:--version" "yarn:--version" "pnpm:--version"; do
  bin="${pm%%:*}"; flag="${pm##*:}"
  v="$(docker run --rm --entrypoint "/usr/bin/$bin" "$IMAGE" "$flag" 2>/dev/null | head -1 || true)"
  [ -n "$v" ] || v="$(docker run --rm --entrypoint "$bin" "$IMAGE" "$flag" 2>/dev/null | head -1 || true)"
  [ -n "$v" ] || { echo "package manager missing: $bin"; exit 1; }
  echo "  $bin: $v"
done

echo "== nonroot user (uid 1001) =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
