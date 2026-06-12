#!/usr/bin/env bash
# Smoke test for a built Pulsar image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs + a writable tmpfs at /pulsar to prove the entrypoint
# relocates ALL of standalone's writable state (data/logs/conf/tmp) correctly, then
# exercises the HTTP admin API and a produce/consume message round-trip.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-pulsar-smoke-$$"
ADMIN="http://127.0.0.1:8080"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs + writable /pulsar tmpfs)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /pulsar:rw,exec,mode=1777,size=1g \
  -p 127.0.0.1:8080:8080 -p 127.0.0.1:6650:6650 "$IMAGE" >/dev/null

# Standalone (broker + bookie + metadata store) takes a while to boot. Poll the
# broker health endpoint -- it returns 200 with body "ok" once the broker is live.
ok=""
for i in $(seq 1 90); do
  if curl -fsS "${ADMIN}/admin/v2/brokers/health" >/dev/null 2>&1; then
    ok=1; break
  fi
  if [ "$i" = 90 ]; then
    echo "pulsar did not become ready"; docker logs "$NAME" 2>&1 | tail -60; exit 1
  fi
  sleep 2
done
echo "broker health: $(curl -fsS "${ADMIN}/admin/v2/brokers/health" 2>/dev/null)"

# The standalone cluster registers itself as "standalone".
echo "clusters: $(curl -fsS "${ADMIN}/admin/v2/clusters" 2>/dev/null)"
curl -fsS "${ADMIN}/admin/v2/clusters" 2>/dev/null | grep -q 'standalone' \
  || { echo "expected 'standalone' cluster"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

# End-to-end messaging: produce then consume a message via the in-image client.
echo "producing a message"
docker exec "$NAME" bin/pulsar-client produce -m "quench" -n 1 \
  persistent://public/default/gate 2>&1 | tail -2 \
  || { echo "produce failed"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

echo "consuming the message"
# -p Earliest: read from the start of the topic so the consume is deterministic
# regardless of subscribe-vs-produce ordering.
out="$(docker exec "$NAME" bin/pulsar-client consume -s sub -p Earliest -n 1 \
  persistent://public/default/gate 2>&1)"
echo "$out" | grep -q "quench" \
  || { echo "consume did not round-trip the message"; echo "$out" | tail -20; exit 1; }
echo "message round-trip OK"

# must run as the nonroot pulsar user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
