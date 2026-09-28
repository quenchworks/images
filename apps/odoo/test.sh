#!/usr/bin/env bash
# Smoke test for a built Odoo image. Usage: test.sh <image-ref> [version]
# Initialises the base module into the QuenchWorks PostgreSQL image, starts the server
# on a read-only root filesystem, and requires /web/health to pass and the login page
# to render. The version is the dated nightly build (19.0.YYYYMMDD).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-odoo-smoke-$$"
NET="$TAG-net"
PG=ghcr.io/quenchworks/images/postgresql@sha256:4c0580fd2968b8d6e8fb008e0eb74b55c54464b041c235842e4d1cc37ed9a445
PW="pg-$$-secret"

cleanup() { docker rm -f "$TAG-pg" "$TAG-app" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

py() { docker run --rm --entrypoint /opt/odoo/venv/bin/python "$IMAGE" -c "$1"; }
ver="$(py 'import importlib.metadata as m; print(m.version("odoo"))')"
echo "odoo $ver"
[ -z "$WANT" ] || [ "$ver" = "${WANT%.*}.post${WANT##*.}" ] || { echo "expected $WANT"; exit 1; }

docker network create "$NET" >/dev/null
# Odoo refuses the user "postgres"; the QuenchWorks image creates POSTGRES_DB only when
# it differs from POSTGRES_USER
docker run -d --name "$TAG-pg" --network "$NET" --network-alias pg \
  -e POSTGRES_USER=odoo -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=odoodb "$PG" >/dev/null
for i in $(seq 1 60); do
  docker exec "$TAG-pg" pg_isready -h 127.0.0.1 -U odoo -d odoodb >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "postgres never ready"; docker logs "$TAG-pg" | tail -20; exit 1; }
  sleep 1
done

db=(--db_host=pg --db_user=odoo "--db_password=$PW" -d odoodb)
out="$(docker run --rm --network "$NET" --read-only --tmpfs /tmp --tmpfs /data:uid=1001,gid=1001 \
  "$IMAGE" --data-dir=/data "${db[@]}" -i base --without-demo=all --stop-after-init 2>&1 || true)"
grep -q "Modules loaded" <<<"$out" || { echo "base init failed:"; tail -30 <<<"$out"; exit 1; }
echo "  base module installed"

docker run -d --name "$TAG-app" --network "$NET" --read-only --tmpfs /tmp \
  --tmpfs /data:uid=1001,gid=1001 -p 127.0.0.1:18069:8069 \
  "$IMAGE" --data-dir=/data "${db[@]}" --db-filter='^odoodb$' >/dev/null
for i in $(seq 1 60); do
  health="$(curl -fsS http://127.0.0.1:18069/web/health 2>/dev/null || true)"
  grep -q '"pass"' <<<"$health" && break
  [ "$i" = 60 ] && { echo "never healthy"; docker logs "$TAG-app" 2>&1 | tail -30; exit 1; }
  sleep 1
done
echo "  /web/health: $health"
page="$(curl -fsS http://127.0.0.1:18069/web/login)"
grep -q 'name="login"' <<<"$page" || { echo "login page did not render"; head -c 500 <<<"$page"; exit 1; }
echo "  /web/login renders the login form"
if docker logs "$TAG-app" 2>&1 | grep -E ' (ERROR|CRITICAL) ' | grep -v 'wkhtmltopdf' | grep -q .; then
  echo "errors in the server log:"; docker logs "$TAG-app" 2>&1 | grep -E ' (ERROR|CRITICAL) ' | head; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (odoo $ver, read-only rootfs, nonroot user: $user)"
