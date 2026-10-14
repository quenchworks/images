#!/usr/bin/env bash
# Smoke test for a built plone image. Usage: test.sh <image-ref> [version]
# Boots on a read-only root with a data volume: the entrypoint adds the admin user and
# creates the Plone site, then plone.restapi must answer /Plone/++api++ with the site
# JSON (anonymous and as admin). A restart must reuse the site from Data.fs.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="plone-smoke-$$"
VOL="plone-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$VOL" >/dev/null 2>&1 || true; }
trap cleanup EXIT
trap 'echo "failed at line $LINENO"; docker logs "$NAME" 2>&1 | tail -30' ERR

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /opt/plone/venv/bin/python "$IMAGE" -c "import importlib.metadata as m; print(m.version('Plone'))")"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "version mismatch: $ver"; exit 1; }

B=http://127.0.0.1:18080
boot() {
  docker run -d --name "$NAME" --read-only --tmpfs /tmp -v "$VOL:/data" -e PLONE_ADMIN_PASSWORD=smoke-pw \
    -p 127.0.0.1:18080:8080 "$IMAGE" >/dev/null
  for _ in $(seq 1 120); do curl -fsS -o /dev/null "$B/" 2>/dev/null && return 0; sleep 2; done
  echo "plone never answered"; docker logs "$NAME" 2>&1 | tail -40; exit 1
}
check_api() {
  local body
  body="$(curl -fsS -H 'Accept: application/json' "$B/Plone/++api++")"
  grep -q '"@id": *"'"$B"'/Plone"' <<<"$body" || { echo "no site JSON: $(head -c 300 <<<"$body")"; exit 1; }
  grep -q '"@type": *"Plone Site"' <<<"$body" || { echo "not a Plone Site: $(head -c 300 <<<"$body")"; exit 1; }
  body="$(curl -fsS -u admin:smoke-pw -H 'Accept: application/json' "$B/Plone/++api++/@users/admin")"
  grep -q '"username": *"admin"' <<<"$body" || { echo "admin not authenticated: $(head -c 300 <<<"$body")"; exit 1; }
}

boot
check_api
logs="$(docker logs "$NAME" 2>&1)"
grep -q 'created Plone site /Plone' <<<"$logs" || { echo "site was not created on first boot"; exit 1; }
if grep -E 'Traceback|CRITICAL' <<<"$logs"; then echo "plone logged errors"; exit 1; fi

docker rm -f "$NAME" >/dev/null
boot
check_api
logs="$(docker logs "$NAME" 2>&1)"
if grep -q 'created Plone site' <<<"$logs"; then echo "site recreated on restart: Data.fs not reused"; exit 1; fi
echo "smoke test passed (plone ${WANT:-$ver}, uid $user, site created, /Plone/++api++ served, reused after restart)"
