#!/usr/bin/env bash
# Smoke test for a built graylog image. Usage: test.sh <image-ref>
# Graylog needs MongoDB + a search store (OpenSearch/Elasticsearch) to fully boot,
# which the CHART wires -- so the image smoke stays at: runs as nonroot 1001,
# a working JRE is present, the server jar is in place, and the jar reports its
# version (the entrypoint's non-server passthrough, no Mongo/OpenSearch needed).
# Full boot is deferred to the chart's kind install gate.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "== runs as nonroot uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "== java runtime present =="
docker run --rm --entrypoint java "$IMAGE" -version

echo "== graylog server jar present =="
docker run --rm --entrypoint sh "$IMAGE" -c 'test -f /opt/graylog/graylog.jar && echo ok' 2>/dev/null \
  || docker run --rm --entrypoint /usr/bin/busybox "$IMAGE" sh -c 'test -f /opt/graylog/graylog.jar && echo ok'

echo "== graylog reports its version (no Mongo/OpenSearch) =="
ver="$(docker run --rm "$IMAGE" version 2>&1 | head -5)"
echo "$ver"
echo "$ver" | grep -qiE '^Graylog [0-9]+\.[0-9]+\.[0-9]+' || { echo "version not reported"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
