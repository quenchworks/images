#!/usr/bin/env bash
# Smoke test for a built SPIRE image. Usage: test.sh <image-ref> [version]
# End to end on the baked demo configs: the server mints a join token, an agent
# attests with it, and a workload (uid 1001) fetches an X.509 SVID for the entry
# registered for it through the agent's Workload API.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="quench-spire-smoke-$$"
NET="$TAG-net"
TD=spiffe://example.org

cleanup() { docker rm -f "$TAG-server" "$TAG-agent" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

for b in spire-server spire-agent; do
  v="$(docker run --rm --entrypoint "/usr/bin/$b" "$IMAGE" --version 2>&1 | tail -1)"
  echo "$b reports: $v"
  case "$v" in ""|*dev*) echo "version not stamped: '$v'"; exit 1 ;; esac
  [ -z "$WANT" ] || [ "$v" = "$WANT" ] || { echo "expected $WANT"; exit 1; }
done

wait_ready() {
  for i in $(seq 1 60); do
    curl -fsS "http://127.0.0.1:$1/ready" >/dev/null 2>&1 && return 0
    sleep 1
  done
  echo "$2 never became ready"; docker logs "$TAG-$2" | tail -30; exit 1
}

docker network create "$NET" >/dev/null
docker run -d --name "$TAG-server" --network "$NET" --network-alias spire-server \
  -p 127.0.0.1:18080:8080 "$IMAGE" >/dev/null
wait_ready 18080 server
srv() { docker exec "$TAG-server" /usr/bin/spire-server "$@"; }

token="$(srv token generate -spiffeID "$TD/node1" | awk '/^Token:/ {print $2}')"
[ -n "$token" ] || { echo "no join token"; exit 1; }
srv entry create -parentID "$TD/node1" -spiffeID "$TD/workload" -selector unix:uid:1001 >/dev/null

docker run -d --name "$TAG-agent" --network "$NET" -p 127.0.0.1:18081:8080 \
  --entrypoint /usr/bin/spire-agent "$IMAGE" run -config /etc/spire/agent/agent.conf -joinToken "$token" >/dev/null
wait_ready 18081 agent

svid=""
for i in $(seq 1 30); do
  svid="$(docker exec "$TAG-agent" /usr/bin/spire-agent api fetch x509 -socketPath /tmp/spire-agent/public/api.sock 2>&1)" \
    && grep -q "$TD/workload" <<<"$svid" && break
  [ "$i" = 30 ] && { echo "workload never got an SVID: $svid"; docker logs "$TAG-agent" | tail -30; exit 1; }
  sleep 2
done
echo "  workload (uid 1001) fetched an X.509 SVID for $TD/workload"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# the registration is in the sqlite datastore under /data/spire: it survives a restart
docker restart "$TAG-server" >/dev/null
wait_ready 18080 server
grep -q "$TD/workload" <<<"$(srv entry show -spiffeID "$TD/workload")" \
  || { echo "entry lost across a server restart"; exit 1; }

echo "smoke test passed (version $v, nonroot user: $user, join token -> agent -> X.509 SVID, entry kept across restart)"
