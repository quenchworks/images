#!/usr/bin/env bash
# Smoke test for a built storm image. Usage: test.sh <image-ref> [version]
# Runs a small cluster from the one image: Storm's own dev ZooKeeper, Nimbus and the UI,
# then asks the UI's REST API for the cluster summary (version) and the Nimbus leader.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="quench-storm-$$"
cleanup() { docker rm -f "$NET-zk" "$NET-nimbus" "$NET-ui" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ver="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$ver" | head -3
[ -z "$WANT" ] || grep -q "^Storm $WANT" <<<"$ver" || { echo "expected Storm $WANT"; exit 1; }

docker network create "$NET" >/dev/null
C=(-c 'storm.zookeeper.servers=["zk"]' -c 'nimbus.seeds=["nimbus"]')
docker run -d --name "$NET-zk" --network "$NET" --network-alias zk "$IMAGE" dev-zookeeper >/dev/null
docker run -d --name "$NET-nimbus" --network "$NET" --network-alias nimbus "$IMAGE" nimbus "${C[@]}" >/dev/null
docker run -d --name "$NET-ui" --network "$NET" -p 8080 "$IMAGE" ui "${C[@]}" -c ui.port=8080 >/dev/null
PORT="$(docker port "$NET-ui" 8080/tcp | head -1 | sed 's/.*://')"

sum=""; nim=""
for i in $(seq 1 90); do
  sum="$(curl -s --max-time 5 "http://localhost:$PORT/api/v1/cluster/summary" || true)"
  nim="$(curl -s --max-time 5 "http://localhost:$PORT/api/v1/nimbus/summary" || true)"
  if grep -q '"stormVersion"' <<<"$sum" && grep -q '"Leader"' <<<"$nim"; then echo "UI and Nimbus leader up after ~$((i * 2))s"; break; fi
  [ "$i" = 90 ] && { echo "cluster did not come up"; echo "$sum"; echo "$nim"; for c in zk nimbus ui; do echo "== $c"; docker logs "$NET-$c" 2>&1 | tail -30; done; exit 1; }
  sleep 2
done
echo "$sum" | head -c 400; echo
[ -z "$WANT" ] || grep -q "\"stormVersion\":\"$WANT\"" <<<"$sum" || { echo "UI reports a different version"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed"
