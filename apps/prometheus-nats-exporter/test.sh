#!/usr/bin/env bash
# Smoke test for a built prometheus-nats-exporter image. Usage: test.sh <image-ref> [version]
# Real scraping: the QuenchWorks nats image runs with JetStream and its monitoring port,
# the exporter (read-only root) scrapes it with the flags Argo Events passes, and its
# /metrics must carry live NATS server and JetStream series.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="pne-smoke-$$"; NATS="$NET-nats"; NAME="$NET-exporter"
cleanup() { docker rm -f "$NAME" "$NATS" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --read-only "$IMAGE" -version 2>&1)"
[ -z "$WANT" ] || echo "$ver" | grep -qF "$WANT" || { echo "version mismatch: $ver"; exit 1; }
docker network create "$NET" >/dev/null
docker run -d --name "$NATS" --network "$NET" --tmpfs /tmp ghcr.io/quenchworks/images/nats:2.15.0 -js -sd /tmp/js -m 8222 >/dev/null
docker run -d --name "$NAME" --network "$NET" --read-only -p 127.0.0.1:17777:7777 "$IMAGE" \
  -connz -routez -subz -varz -prefix=nats -use_internal_server_id -jsz=all "http://$NATS:8222" >/dev/null
ok=0
for _ in $(seq 1 30); do curl -fsS http://127.0.0.1:17777/metrics 2>/dev/null | grep -q '^nats_varz_' && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] || { echo "no nats_varz metrics"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
m="$(curl -fsS http://127.0.0.1:17777/metrics)"
echo "$m" | grep -q '^nats_varz_connections' || { echo "missing varz connections"; exit 1; }
echo "$m" | grep -q '^nats_.*jetstream\|^nats_jetstream_\|_jsz_' || { echo "missing JetStream series"; echo "$m" | grep -o '^nats_[a-z_]*' | sort -u | head -30; exit 1; }
echo "smoke test passed (prometheus-nats-exporter ${WANT:-?}, uid $user, live varz and JetStream metrics)"
