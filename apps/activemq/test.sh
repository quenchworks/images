#!/usr/bin/env bash
# Smoke test for a built ActiveMQ Classic image. Usage: test.sh <image-ref> [version]
# Produces persistent messages with ActiveMQ's own CLI, restarts the broker, and
# requires every message to come back out of KahaDB.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-activemq-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$NAME" "$IMAGE" >/dev/null
# docker logs keeps the lines from before a restart, so wait for the Nth start
wait_started() {
  for i in $(seq 1 90); do
    [ "$(docker logs "$NAME" 2>&1 | grep -cE "Apache ActiveMQ [0-9.]+ .* started")" -ge "$1" ] && return 0
    sleep 1
  done
  echo "broker never started"; docker logs "$NAME" 2>&1 | tail -40; exit 1
}
wait_started 1
line="$(docker logs "$NAME" 2>&1 | grep -oE "Apache ActiveMQ [0-9.]+ \([^)]*\) started" | tail -1)"
echo "$line"
[ -z "$WANT" ] || grep -q "ActiveMQ $WANT " <<<"$line" || { echo "expected $WANT"; exit 1; }

cli() {
  docker exec "$NAME" /usr/lib/jvm/java-21-openjdk/bin/java \
    -Dactivemq.home=/opt/activemq -Dactivemq.base=/opt/activemq -Dactivemq.conf=/opt/activemq/conf \
    -Dactivemq.data=/tmp/cli -jar /opt/activemq/bin/activemq.jar "$@" \
    --brokerUrl tcp://localhost:61616 --destination queue://smoke --messageCount 5 2>&1
}
out="$(cli producer --persistent true || true)"
grep -qiE "produced:? *5|5 messages" <<<"$out" || { echo "producer: $out" | tail -15; exit 1; }
echo "  produced 5 persistent messages"

docker restart "$NAME" >/dev/null
wait_started 2
out="$(cli consumer --receiveTimeout 20000 || true)"
grep -qiE "consumed:? *5|5 messages" <<<"$out" || { echo "consumer: $out" | tail -15; exit 1; }
echo "  consumed all 5 after a broker restart (KahaDB)"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed ($line, nonroot user: $user, persistent messages across a restart)"
