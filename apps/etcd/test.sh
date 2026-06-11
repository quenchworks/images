#!/usr/bin/env bash
# Smoke test for a built etcd image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-etcd-smoke-$$"
EP="http://127.0.0.1:2379"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" \
  -e ETCD_LISTEN_CLIENT_URLS="http://0.0.0.0:2379" \
  -e ETCD_ADVERTISE_CLIENT_URLS="$EP" \
  "$IMAGE" >/dev/null

# wait for the single-node cluster to report healthy
for i in $(seq 1 30); do
  if docker exec "$NAME" etcdctl --endpoints="$EP" endpoint health 2>/dev/null | grep -q healthy; then
    break
  fi
  [ "$i" = 30 ] && { echo "etcd did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "healthy; checking put/get"
docker exec "$NAME" etcdctl --endpoints="$EP" put qw hello >/dev/null
[ "$(docker exec "$NAME" etcdctl --endpoints="$EP" get qw --print-value-only)" = "hello" ] \
  || { echo "put/get failed"; exit 1; }

# must run as the nonroot etcd user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" etcd --version | head -1
echo "smoke test passed (nonroot user: $user)"
