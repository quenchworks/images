#!/usr/bin/env bash
# Smoke test for a built OpenSearch Dashboards image. Usage: test.sh <image-ref> [version]
# Runs it on a read-only root against the catalog OpenSearch image, waits for the status
# API to report the OpenSearch plugin available, then writes an index pattern through the
# saved objects API and reads it back.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
OS_IMAGE="${OPENSEARCH_IMAGE:-ghcr.io/quenchworks/images/opensearch:${WANT:-3.8.0}}"
NAME="osd-smoke-$$"
NET="$NAME-net"
cleanup() { docker rm -f "$NAME" "$NAME-os" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$NAME-os" --network "$NET" --network-alias opensearch "$OS_IMAGE" >/dev/null
docker run -d --name "$NAME" --network "$NET" --read-only --tmpfs /tmp \
  --tmpfs /usr/share/opensearch-dashboards/data:rw,uid=1001,gid=1001 \
  -p 127.0.0.1:15601:5601 "$IMAGE" >/dev/null

ok=0
for _ in $(seq 1 120); do
  st="$(curl -fsS http://127.0.0.1:15601/api/status 2>/dev/null || true)"
  [ "$(jq -r '.status.overall.state // empty' <<<"$st")" = green ] && { ok=1; break; }
  sleep 2
done
[ "$ok" = 1 ] || { echo "status never green"; jq -c '.status.overall // .' <<<"${st:-null}" || true; docker logs "$NAME" | tail -40; exit 1; }
ver="$(jq -r '.version.number' <<<"$st")"
echo "opensearch-dashboards $ver, status green"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }

curl -fsS -X POST http://127.0.0.1:15601/api/saved_objects/index-pattern/quench-smoke \
  -H 'osd-xsrf: true' -H 'Content-Type: application/json' \
  -d '{"attributes":{"title":"quench-smoke-*"}}' >/dev/null
got="$(curl -fsS http://127.0.0.1:15601/api/saved_objects/index-pattern/quench-smoke)"
[ "$(jq -r .attributes.title <<<"$got")" = 'quench-smoke-*' ] || { echo "saved object not read back: $got"; exit 1; }
echo "saved object round-trip through OpenSearch ok"
echo "smoke test passed"
