#!/usr/bin/env bash
# Smoke test for a built Apache Artemis image. Usage: test.sh <image-ref> [version]
# Mounts a users file, sends persistent messages with Artemis' own CLI, restarts the
# broker, requires every message back out of the journal, and requires a wrong
# password to be refused.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-artemis-smoke-$$"
# Docker Desktop cannot bind-mount /tmp
ETC="$(mktemp -d "$HOME/.quench-artemis-XXXXXX")"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$ETC"; }
trap cleanup EXIT

echo 'smoke = smokepw' > "$ETC/artemis-users.properties"
echo 'amq = smoke' > "$ETC/artemis-roles.properties"
chmod 0644 "$ETC"/*

docker run -d --name "$NAME" \
  -v "$ETC/artemis-users.properties:/opt/artemis-instance/etc/artemis-users.properties:ro" \
  -v "$ETC/artemis-roles.properties:/opt/artemis-instance/etc/artemis-roles.properties:ro" \
  "$IMAGE" >/dev/null
# docker logs keeps the lines from before a restart, so wait for the Nth start
wait_started() {
  for i in $(seq 1 90); do
    [ "$(docker logs "$NAME" 2>&1 | grep -c "AMQ221007: Server is now active")" -ge "$1" ] && return 0
    sleep 1
  done
  echo "broker never started"; docker logs "$NAME" 2>&1 | tail -40; exit 1
}
wait_started 1
line="$(docker logs "$NAME" 2>&1 | grep -oE "Apache Artemis Message Broker version [0-9.]+" | tail -1)"
echo "$line"
[ -z "$WANT" ] || grep -q "version $WANT\$" <<<"$line" || { echo "expected $WANT"; exit 1; }

cli() {
  docker exec "$NAME" /usr/lib/jvm/java-21-openjdk/bin/java \
    -Dartemis.home=/opt/artemis -Dartemis.instance=/opt/artemis-instance -Djava.io.tmpdir=/tmp \
    -classpath /opt/artemis/lib/artemis-boot.jar org.apache.activemq.artemis.boot.Artemis \
    "$@" --url tcp://localhost:61616 --destination queue://smoke 2>&1
}
out="$(cli producer --user smoke --password smokepw --message-count 5 || true)"
grep -q "Produced: 5 messages" <<<"$out" || { echo "producer: $out" | tail -15; exit 1; }
echo "  produced 5 persistent messages"

docker restart "$NAME" >/dev/null
wait_started 2
out="$(cli consumer --user smoke --password smokepw --message-count 5 --receive-timeout 20000 || true)"
grep -q "Consumed: 5 messages" <<<"$out" || { echo "consumer: $out" | tail -15; exit 1; }
echo "  consumed all 5 after a broker restart (journal)"

out="$(cli producer --user smoke --password wrong --message-count 1 || true)"
grep -q "Produced: 1 messages" <<<"$out" && { echo "a wrong password was accepted"; exit 1; }
grep -qiE "AMQ229031|security|authentication|credentials" <<<"$out" || { echo "wrong password: $out" | tail -15; exit 1; }
echo "  a wrong password is refused"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed ($line, nonroot user: $user, persistent messages across a restart)"
