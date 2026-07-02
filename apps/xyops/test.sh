#!/usr/bin/env bash
# Smoke test for a built xyOps image. Usage: test.sh <image-ref>
#
# Boots xyOps against its default embedded SQLite store. xyOps reads its base config
# from the baked /opt/xyops/conf and applies XYOPS_* env overrides; the only required
# override is a non-empty secret_key (supplied by the chart from a Secret in production).
# The web UI + API answer on 5522 (http). The image tree is owned by uid 1001 and the
# default container layer is writable, so a standalone boot writes data/logs/temp in place
# (in Kubernetes a PVC overmounts /opt/xyops/data).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-xyops-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" \
  --tmpfs /tmp:rw,mode=1777 \
  -e XYOPS_secret_key=smoke-test-secret-key-not-for-production \
  -p 127.0.0.1:5522:5522 "$IMAGE" >/dev/null

# xyOps must answer on 5522 (200, or a redirect) once pixl-server has started its web
# listener and initialised the SQLite storage.
code=""
for i in $(seq 1 60); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:5522/ 2>/dev/null || true)"
  case "$code" in
    200|301|302) break ;;
  esac
  [ "$i" = 60 ] && { echo "xyops did not start serving (last code: '$code')"; docker logs "$NAME"; exit 1; }
  sleep 2
done
echo "GET / -> $code"

# the served root must actually be the xyOps app UI
curl -fsS http://127.0.0.1:5522/ 2>/dev/null | grep -qi 'xyops' \
  || { echo "served page does not look like xyOps"; docker logs "$NAME" | tail -20; exit 1; }

# the JSON API must be live (the login UI calls it)
ping="$(curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:5522/api/app/ping 2>/dev/null || true)"
echo "GET /api/app/ping -> $ping"
[ "$ping" = "200" ] || { echo "xyOps API not responding"; docker logs "$NAME" | tail -20; exit 1; }

# must run as the nonroot xyops user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
