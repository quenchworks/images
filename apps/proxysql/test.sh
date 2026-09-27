#!/usr/bin/env bash
# Smoke test for a built ProxySQL image. Usage: test.sh <image-ref> [version]
#
# ProxySQL speaks the MySQL protocol, so the test uses a real client, the one in
# our own mariadb image, run in the proxysql container's network namespace:
#   1. proxysql --version reports the version.
#   2. The image boots as uid 1001 on a READ-ONLY rootfs with only the datadir
#      writable, the way the chart runs it.
#   3. The admin interface (6032) answers SQL as `admin`, which ProxySQL allows
#      from localhost only, and reports the running version.
#   4. The admin tables work: a mysql_servers row is inserted, loaded to
#      runtime and read back from runtime_mysql_servers.
#   5. The client interface (6033) completes a MySQL handshake: an unknown user
#      is refused with ProxySQL's own access-denied error (1045).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-proxysql-smoke-$$"
CLIENT="${MYSQL_CLIENT_IMAGE:-ghcr.io/quenchworks/images/mariadb:11.8.9}"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ver_out="$(docker run --rm --entrypoint /usr/bin/proxysql "$IMAGE" --version 2>&1 | grep "ProxySQL version" || true)"
echo "$ver_out"
grep -qE 'ProxySQL version [0-9]+\.[0-9]+\.[0-9]+' <<<"$ver_out" || { echo "no version"; exit 1; }
if [ -n "${2:-}" ]; then
  grep -qF "version ${2}" <<<"$ver_out" || { echo "expected version $2"; exit 1; }
fi

docker run -d --name "$NAME" --read-only \
  --tmpfs /var/lib/proxysql:rw,uid=1001,gid=1001 --tmpfs /tmp:rw,mode=1777 \
  "$IMAGE" >/dev/null

sql() { docker run --rm --network "container:$NAME" --entrypoint /usr/bin/mariadb "$CLIENT" \
  -h 127.0.0.1 -P "$1" -u "$2" -p"$3" --skip-ssl -N -B -e "$4"; }

out=""
for i in $(seq 1 45); do
  out="$(sql 6032 admin admin 'SELECT variable_value FROM global_variables WHERE variable_name="admin-version"' 2>/dev/null || true)"
  [ -n "$out" ] && break
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "container died during startup:"; docker logs "$NAME"; exit 1; }
  [ "$i" = 45 ] && { echo "admin interface never answered"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "admin-version: $out"
grep -qE '^[0-9]+\.[0-9]+\.[0-9]+' <<<"$out" || { echo "admin-version looks wrong"; exit 1; }

rt="$(sql 6032 admin admin "INSERT INTO mysql_servers (hostgroup_id, hostname, port) VALUES (0, 'db.quench.invalid', 3306); LOAD MYSQL SERVERS TO RUNTIME; SELECT hostname FROM runtime_mysql_servers")"
grep -qx 'db.quench.invalid' <<<"$rt" || { echo "runtime_mysql_servers did not return the new server: $rt"; exit 1; }
echo "admin tables: server loaded to runtime"

err="$(sql 6033 nobody wrong 'SELECT 1' 2>&1 || true)"
echo "$err"
grep -q 'ERROR 1045' <<<"$err" || { echo "client port did not refuse an unknown user with 1045"; exit 1; }

# Not [ERROR] in general: step 5's refused login and the monitor's failed lookups
# of db.quench.invalid are logged as [ERROR] by design.
if docker logs "$NAME" 2>&1 | grep -qE 'Read-only file system|[Pp]ermission denied|unable to open database'; then
  echo "found errors in logs:"; docker logs "$NAME"; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only rootfs, admin SQL and client handshake)"
