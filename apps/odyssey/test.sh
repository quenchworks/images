#!/usr/bin/env bash
# Smoke test for a built Odyssey image. Usage: test.sh <image-ref> [version]
# Odyssey in front of the QuenchWorks postgresql image: a client connects to the pooler on 6432
# and its query reaches PostgreSQL through a pooled server connection.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
PG=ghcr.io/quenchworks/images/postgresql:18.6
ID="odyssey-smoke-$$"
cleanup() { docker rm -f "$ID-pg" "$ID-od" >/dev/null 2>&1 || true; docker network rm "$ID" >/dev/null 2>&1 || true; rm -rf "${conf:-}"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version)"
echo "$v"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

# config under $PWD: Docker Desktop cannot mount /tmp
conf="$(mktemp -d "${PWD}/.odyssey-smoke.XXXXXX")"
cat > "$conf/odyssey.conf" <<C
daemonize no
log_to_stdout yes
log_format "%p %t %l [%i %s] (%c) %m\\n"
unix_socket_dir "/tmp"
unix_socket_mode "0644"
workers 1
listen {
	host "*"
	port 6432
	tls "disable"
}
storage "pg" {
	type "remote"
	host "$ID-pg"
	port 5432
}
database default {
	user default {
		authentication "none"
		storage "pg"
		storage_user "postgres"
		storage_password "smoke-pass"
		pool "session"
	}
}
C
chmod -R a+rX "$conf"

docker network create "$ID" >/dev/null
docker run -d --name "$ID-pg" --network "$ID" -e POSTGRES_PASSWORD=smoke-pass "$PG" >/dev/null
docker run -d --name "$ID-od" --network "$ID" --read-only --tmpfs /tmp -v "$conf:/etc/odyssey:ro" "$IMAGE" >/dev/null

ok=0
for _ in $(seq 1 60); do
  out="$(docker run --rm --network "$ID" --entrypoint psql "$PG" -h "$ID-od" -p 6432 -U postgres -d postgres -tAc "select 'through odyssey ' || current_setting('server_version_num')" 2>/dev/null || true)"
  grep -q '^through odyssey ' <<<"$out" && { ok=1; break; }
  sleep 2
done
[ "$ok" = 1 ] || { echo "query never came back through the pooler"; docker logs "$ID-od" 2>&1 | tail -20; docker logs "$ID-pg" 2>&1 | tail -5; exit 1; }
echo "$out"

# TLS for clients: Odyssey built against OpenSSL 4.0 segfaulted on the first TLS client, so a
# TLS query must come back and the container must still be running afterwards
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj "/CN=$ID-od" \
  -keyout "$conf/tls.key" -out "$conf/tls.crt" >/dev/null 2>&1
chmod a+r "$conf/tls.key" "$conf/tls.crt"
sed -i 's|tls "disable"|tls "require"\n\ttls_cert_file "/etc/odyssey/tls.crt"\n\ttls_key_file "/etc/odyssey/tls.key"|' "$conf/odyssey.conf"
docker restart "$ID-od" >/dev/null
ok=0
for _ in $(seq 1 30); do
  out="$(docker run --rm --network "$ID" --entrypoint psql -e PGSSLMODE=require "$PG" -h "$ID-od" -p 6432 -U postgres -d postgres -tAc "select 'tls through odyssey'" 2>/dev/null || true)"
  [ "$out" = "tls through odyssey" ] && { ok=1; break; }
  sleep 2
done
[ "$ok" = 1 ] || { echo "TLS query never came back"; docker logs "$ID-od" 2>&1 | tail -10; exit 1; }
sleep 2
[ "$(docker inspect -f '{{.State.Running}} {{.RestartCount}}' "$ID-od")" = "true 0" ] || { echo "odyssey died after a TLS client: $(docker inspect -f '{{.State.ExitCode}}' "$ID-od")"; exit 1; }
plain="$(docker run --rm --network "$ID" --entrypoint psql -e PGSSLMODE=disable "$PG" -h "$ID-od" -p 6432 -U postgres -d postgres -tAc "select 1" 2>&1 || true)"
[ "$plain" != 1 ] || { echo "plain client accepted with tls require"; exit 1; }
echo "smoke test passed: pooled query, TLS query with the process still up, plain refused under require, user 1001"
