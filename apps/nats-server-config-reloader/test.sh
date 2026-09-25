#!/usr/bin/env bash
# Smoke test for a built nats-server-config-reloader image. Usage: test.sh <image-ref> [version]
# The pod layout Argo Events uses: the QuenchWorks nats image and the reloader share a pid
# namespace, a pid file and the config. Editing the config must make the reloader signal
# nats-server, and the server must log that it reloaded.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NATS="reloader-smoke-nats-$$"; NAME="reloader-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'docker rm -f "$NAME" "$NATS" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --read-only "$IMAGE" -version 2>&1 || true)"
[ -z "$WANT" ] || echo "$ver" | grep -qF "$WANT" || { echo "version mismatch: $ver"; exit 1; }
mkdir -p "$WORK/conf" "$WORK/run"
printf 'port: 4222\nhttp_port: 8222\nmax_payload: 1048576\n' > "$WORK/conf/nats.conf"
chmod 0777 "$WORK/conf" "$WORK/run"; chmod 0666 "$WORK/conf/nats.conf"
docker run -d --name "$NATS" -v "$WORK/conf:/etc/nats-config" -v "$WORK/run:/var/run/nats" \
  ghcr.io/quenchworks/images/nats:2.15.0 --config /etc/nats-config/nats.conf -P /var/run/nats/nats.pid >/dev/null
for _ in $(seq 1 30); do [ -s "$WORK/run/nats.pid" ] && break; sleep 1; done
[ -s "$WORK/run/nats.pid" ] || { echo "nats wrote no pid file"; docker logs "$NATS" 2>&1 | tail; exit 1; }
docker run -d --name "$NAME" --read-only --pid "container:$NATS" \
  -v "$WORK/conf:/etc/nats-config:ro" -v "$WORK/run:/var/run/nats:ro" "$IMAGE" \
  -pid /var/run/nats/nats.pid -config /etc/nats-config/nats.conf >/dev/null
sleep 3
printf 'port: 4222\nhttp_port: 8222\nmax_payload: 2097152\n' > "$WORK/conf/nats.conf"
ok=0
for _ in $(seq 1 30); do docker logs "$NATS" 2>&1 | grep -qi 'reloaded' && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] || { echo "nats never reloaded"; echo "--- reloader"; docker logs "$NAME" 2>&1 | tail -15; echo "--- nats"; docker logs "$NATS" 2>&1 | tail -10; exit 1; }
echo "smoke test passed (nats-server-config-reloader ${WANT:-?}, uid $user, config edit reloaded nats-server)"
