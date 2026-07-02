#!/usr/bin/env bash
# Smoke test for a built SkyWalking OAP image. Usage: test.sh <image-ref>
#
# SkyWalking 10.2+ removed the H2 in-memory store; OAP now defaults to BanyanDB
# (SW_STORAGE:banyandb) and cannot fully start without an external storage backend
# -- that is the CHART's kind gate to prove (it brings up BanyanDB). Here we prove
# the IMAGE is sound: nonroot uid 1001, the bundled JRE runs, the OAP classpath +
# config are present, the entrypoint parses, and -- the strong check -- OAP's main
# class actually launches on that classpath and begins initialization before it
# fails to reach storage.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-skywalking-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== 1. runs as nonroot uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "user: $user"

echo "== 2. JRE runs, OAP classpath + config present, entrypoint parses =="
docker run --rm --entrypoint sh "$IMAGE" -c '
  set -e
  "$JAVA_HOME/bin/java" -version
  ls /opt/skywalking/oap-libs/server-starter-*.jar >/dev/null
  test -f /opt/skywalking/config/application.yml
  sh -n /usr/bin/docker-entrypoint.sh
  echo "classpath + config OK"
'

echo "== 3. OAP boots its core modules and binds the gRPC receiver (port 11800) =="
# No storage backend here, so OAP will eventually fail/exit on storage init -- that
# is expected and deferred to the chart's kind gate (which brings up BanyanDB).
# We require the stronger signal that OAP parsed application.yml, loaded its core
# modules, and bound the gRPC server on 11800 -- proof the OAP server really runs.
docker run -d --name "$NAME" "$IMAGE" >/dev/null
ok=0
for i in $(seq 1 60); do
  if docker logs "$NAME" 2>&1 | grep -Eiq "listening on 11800"; then
    echo "OAP gRPC receiver bound on 11800 after ~${i}s"; ok=1; break
  fi
  sleep 1
done
if [ "$ok" != 1 ]; then
  echo "OAP did not bind its gRPC receiver; logs:"; docker logs "$NAME" 2>&1 | tail -60; exit 1
fi

echo "logs (tail):"; docker logs "$NAME" 2>&1 | tail -12
echo "smoke test passed (nonroot uid: $user; OAP booted + bound gRPC 11800 on the bundled JRE)"
