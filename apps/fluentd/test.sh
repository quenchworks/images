#!/usr/bin/env bash
# Smoke test for a built Fluentd image. Usage: test.sh <image-ref> [version]
# Sends an event to the default config's HTTP input and requires the stdout output to
# print it with its tag.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-fluentd-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ver="$(docker run --rm "$IMAGE" --version 2>&1 | tail -1)"
echo "reported: $ver"
[ -z "$WANT" ] || [ "$ver" = "fluentd $WANT" ] || { echo "expected fluentd $WANT"; exit 1; }

docker run -d --name "$NAME" -p 127.0.0.1:19880:9880 "$IMAGE" >/dev/null
for i in $(seq 1 60); do
  docker logs "$NAME" 2>&1 | grep -q "fluentd worker is now running" && break
  [ "$i" = 60 ] && { echo "fluentd never started"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done

mark="quench-$$"
for i in $(seq 1 20); do
  curl -fsS -X POST -d "json={\"msg\":\"$mark\"}" http://127.0.0.1:19880/smoke.test && break
  [ "$i" = 20 ] && { echo "HTTP input never accepted"; docker logs "$NAME" | tail -20; exit 1; }
  sleep 1
done
for i in $(seq 1 20); do
  docker logs "$NAME" 2>&1 | grep "smoke.test" | grep -q "$mark" && break
  [ "$i" = 20 ] && { echo "event never reached stdout"; docker logs "$NAME" | tail -20; exit 1; }
  sleep 1
done
echo "  HTTP input -> stdout output: event smoke.test delivered"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed ($ver, nonroot user: $user, HTTP input to stdout)"
