#!/usr/bin/env bash
# Smoke test for a built n8n image. Usage: test.sh <image-ref>
#
# Boots n8n against its default standalone sqlite DB (stored under ~/.n8n). We point HOME at
# a writable tmpfs so no PVC is needed for the smoke test. n8n serves the editor/API on 5678.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-n8n-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# n8n listens on 5678. It writes its sqlite DB + config + encryption key under ~/.n8n
# (HOME=/home/node in the image); back that with a writable tmpfs so a standalone boot works
# without a PVC. N8N_DIAGNOSTICS/telemetry off keeps the smoke test hermetic.
docker run -d --name "$NAME" \
  --tmpfs /home/node:rw,mode=0777,uid=1001,gid=1001 \
  --tmpfs /tmp:rw,mode=1777 \
  -e N8N_DIAGNOSTICS_ENABLED=false \
  -e N8N_VERSION_NOTIFICATIONS_ENABLED=false \
  -p 127.0.0.1:5678:5678 "$IMAGE" >/dev/null

# n8n must answer on 5678 (200, or a 3xx redirect to the setup/login page) once it has
# booted + run migrations. First boot generates the encryption key + migrates sqlite, which
# can take a bit, so we poll generously.
code=""
for i in $(seq 1 90); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:5678/ 2>/dev/null || true)"
  case "$code" in
    200|301|302|307) break ;;
  esac
  if [ "$i" = 90 ]; then echo "n8n did not start serving (last code: '$code')"; docker logs "$NAME" | tail -40; exit 1; fi
  sleep 2
done
echo "GET / -> $code"

# The healthz endpoint is the authoritative "server is up" probe.
health="$(curl -fsS http://127.0.0.1:5678/healthz 2>/dev/null || true)"
echo "GET /healthz -> $health"
echo "$health" | grep -qi 'ok' \
  || { echo "healthz did not report ok"; docker logs "$NAME" | tail -40; exit 1; }

# must run as the nonroot node user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# Precondition of apps/n8n/vex.openvex.json (simple-git): the image ships no git executable,
# so no bypassed git option can ever run. Fail if one appears anywhere. No apostrophes in
# this single-quoted script.
docker run --rm --entrypoint /usr/bin/node "$IMAGE" -e '
const fs = require("fs"), path = require("path"), hits = [];
(function walk(d) {
  let es; try { es = fs.readdirSync(d, { withFileTypes: true }); } catch { return; }
  for (const e of es) {
    const p = path.join(d, e.name);
    if (e.isDirectory()) { if (!["/proc", "/sys", "/dev"].includes(p)) walk(p); }
    else if (e.name === "git") hits.push(p);
  }
})("/");
if (hits.length) { console.error("git in image, the simple-git VEX no longer holds:", hits); process.exit(1); }
require("/opt/n8n/node_modules/simple-git").simpleGit("/").version().then(v => {
  if (v.installed) { console.error("simple-git found git:", JSON.stringify(v)); process.exit(1); }
  console.log("no git executable in image; simple-git reports installed=false");
});
'

echo "smoke test passed (nonroot user: $user)"
