#!/usr/bin/env bash
# Smoke test for a built Sozu image. Usage: test.sh <image-ref> [version]
# Starts Sozu with its default config, then reconfigures it at runtime over the command
# socket (cluster, backend, HTTP frontend) and proxies a request to a backend container.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="quench-sozu-$$"; SZ="quench-sozu-smoke-$$"; BE="quench-sozu-be-$$"
cleanup() { docker rm -f "$SZ" "$BE" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" --version)"
echo "sozu: $ver"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$ver" || { echo "expected $WANT"; exit 1; }

docker network create "$NET" >/dev/null
# backend: our caddy image answering every request with a fixed body
docker run -d --name "$BE" --network "$NET" --entrypoint /usr/bin/caddy \
  ghcr.io/quenchworks/images/caddy:2.11.9 respond --listen :8000 quench-backend >/dev/null
docker run -d --name "$SZ" --network "$NET" -p 127.0.0.1::8080 "$IMAGE" >/dev/null

sz() { docker exec "$SZ" /usr/bin/sozu -c /etc/sozu/config.toml "$@"; }
for i in $(seq 1 30); do
  sz status >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { echo "sozu did not come up"; docker logs "$SZ"; exit 1; }
  sleep 1
done
sz status

be_ip="$(docker inspect "$BE" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')"
case "$be_ip" in *.*.*.*) ;; *) echo "backend has no IP ($be_ip)"; docker logs "$BE"; exit 1 ;; esac
sz cluster add --id smoke --load-balancing-policy round_robin
sz backend add --id smoke --backend-id smoke-0 --address "$be_ip:8000"
sz frontend http add --address 0.0.0.0:8080 --hostname smoke.local id smoke

port="$(docker port "$SZ" 8080/tcp | head -1 | sed 's/.*://')"
for i in $(seq 1 15); do
  out="$(curl -s -H 'Host: smoke.local' "http://127.0.0.1:$port/" || true)"
  [ "$out" = quench-backend ] && break
  [ "$i" = 15 ] && { echo "proxying failed, got: $out"; docker logs "$SZ" | tail -30; exit 1; }
  sleep 1
done
echo "proxied: $out"
code="$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: unknown.local' "http://127.0.0.1:$port/")"
[ "$code" = 404 ] || { echo "unknown host should get 404, got $code"; exit 1; }

echo "smoke test passed ($ver, nonroot user: $user)"
