#!/usr/bin/env bash
# Smoke test for a built Ghost image. Usage: test.sh <image-ref>
#
# Boots Ghost against its default standalone sqlite DB. The published image defaults to
# NODE_ENV=production, whose sqlite path lives under the content dir; we point both the
# content dir and a development-mode boot at a writable tmpfs so no PVC is needed for the
# smoke test. Ghost serves its site (and the /ghost admin) on 2368.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-ghost-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# Ghost listens on 2368. content + tmp must be writable for the nonroot user on a
# read-only rootfs; mount tmpfs at both and point Ghost's content path at the writable
# one. NODE_ENV=development selects the default sqlite DB (no external MySQL needed).
docker run -d --name "$NAME" \
  --tmpfs /var/lib/ghost/content:rw,uid=1001,gid=1001,mode=0755 \
  --tmpfs /tmp:rw,mode=1777 \
  -e NODE_ENV=development \
  -e paths__contentPath=/var/lib/ghost/content \
  -e url=http://127.0.0.1:2368 \
  -p 127.0.0.1:2368:2368 "$IMAGE" >/dev/null

# Ghost must answer on 2368 (200, or a 301/302 redirect) once it has booted + migrated.
code=""
for i in $(seq 1 60); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:2368/ 2>/dev/null || true)"
  case "$code" in
    200|301|302) break ;;
  esac
  [ "$i" = 60 ] && { echo "ghost did not start serving (last code: '$code')"; docker logs "$NAME"; exit 1; }
  sleep 2
done
echo "GET / -> $code"

# the booted server must actually be Ghost: its admin redirect / X-Powered-By or the
# homepage markup carries the Ghost signature.
hdrs="$(curl -fsSI http://127.0.0.1:2368/ghost/ 2>/dev/null || true)"
echo "$hdrs" | grep -qi 'ghost' \
  || curl -fsS http://127.0.0.1:2368/ 2>/dev/null | grep -qi 'ghost' \
  || { echo "served page does not look like Ghost"; docker logs "$NAME" | tail -20; exit 1; }

# must run as the nonroot ghost user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
