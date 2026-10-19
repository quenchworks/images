#!/usr/bin/env bash
# Smoke test for a built Netdata image. Usage: test.sh <image-ref> [version]
# Runs the image nonroot on a READ-ONLY rootfs (tmpfs for its state dirs and /tmp) and
# proves the agent collects: /api/v1/info answers (with the expected version when given),
# /api/v1/charts lists system.cpu, the Prometheus export carries netdata_system_cpu,
# and the image ships no web dashboard.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
VER="${2:-}"
NAME="quench-netdata-smoke-$$"
PORT=19999

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

t="rw,uid=1001,gid=1001,mode=0755"
docker run -d --name "$NAME" --read-only \
  --tmpfs "/var/lib/netdata:$t" --tmpfs "/var/cache/netdata:$t" \
  --tmpfs "/var/log/netdata:$t" --tmpfs "/run/netdata:$t" --tmpfs /tmp:rw,mode=1777 \
  -p "127.0.0.1:${PORT}:19999" "$IMAGE" >/dev/null

info=""
for i in $(seq 1 60); do
  info="$(curl -fsS "http://127.0.0.1:${PORT}/api/v1/info" 2>/dev/null)" && break
  info=""
  sleep 1
done
[ -n "$info" ] || { echo "/api/v1/info never answered"; docker logs "$NAME"; exit 1; }
echo "info: $(grep -o '"version": *"[^"]*"' <<<"$info" | head -1)"
if [ -n "$VER" ]; then
  grep -q "\"version\": *\"v${VER}\"" <<<"$info" || { echo "version v${VER} not in /api/v1/info"; exit 1; }
fi

charts=""
for i in $(seq 1 30); do
  charts="$(curl -fsS "http://127.0.0.1:${PORT}/api/v1/charts" 2>/dev/null || true)"
  grep -q '"system.cpu"' <<<"$charts" && break
  sleep 1
done
grep -q '"system.cpu"' <<<"$charts" || { echo "system.cpu missing from /api/v1/charts"; docker logs "$NAME"; exit 1; }
echo "charts: system.cpu present ($(grep -o '"[a-z_]*\.[a-z_0-9]*": *{' <<<"$charts" | wc -l) charts)"

# the export only lists a chart once it has collected a full point; retry briefly
prom=""
for i in $(seq 1 30); do
  prom="$(curl -fsS "http://127.0.0.1:${PORT}/api/v1/allmetrics?format=prometheus" 2>/dev/null || true)"
  grep -q '^netdata_system_cpu' <<<"$prom" && break
  sleep 1
done
grep -q '^netdata_system_cpu' <<<"$prom" || { echo "netdata_system_cpu missing from the Prometheus export"; exit 1; }
echo "prometheus: netdata_system_cpu present"

docker exec "$NAME" test ! -e /usr/share/netdata/web/index.html || { echo "a web dashboard is in the image"; exit 1; }

if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system'; then
  echo "read-only rootfs errors:"; docker logs "$NAME" 2>&1 | grep -iE 'read-only file system' | head; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot user: $user)"
