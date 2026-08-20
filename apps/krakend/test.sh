#!/usr/bin/env bash
# Smoke test for a built KrakenD CE image.
# Usage: test.sh <image-ref> [expected-version]
#
# The image ships a bootable default config at /etc/krakend/krakend.json and the
# entrypoint already points at it, so a bare `docker run` must bring up a working
# gateway. That is the whole point of baking the config: if this test needed a
# mounted config to boot, a plain `docker run` of the published image would not.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
NAME="quench-krakend-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE with the baked default config"
# no args, no mounts: entrypoint is `krakend run -c /etc/krakend/krakend.json`
docker run -d --name "$NAME" -p 127.0.0.1:8080:8080 "$IMAGE" >/dev/null

# lura's router/gin exposes /__health unless disable_health is set; it returns
# {"status":"ok",...}. This is the real "the gateway is serving" signal.
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/__health 2>/dev/null | grep -q '"status":"ok"'; then
    break
  fi
  [ "$i" = 30 ] && { echo "krakend did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "healthy: /__health returned status ok"

# A listening socket is not a working router. The default config declares no
# endpoints, so gin must answer 404 (not connection-refused, not 502) on an
# undeclared route -- that proves the HTTP engine and not just the port is up.
code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/no-such-endpoint)"
[ "$code" = "404" ] || { echo "expected 404 on an undeclared route, got '$code'"; docker logs "$NAME"; exit 1; }
echo "router live: undeclared route returned 404"

# The version must be stamped onto lura's core.KrakendVersion, whose default is
# "undefined". Two quirks: the entrypoint carries args, so `version` needs
# --entrypoint; and krakend writes `version` (and `check`) to STDERR, not stdout,
# so the 2>&1 is load-bearing -- without it awk reads an empty stream and this
# test fails on a perfectly good image.
ver="$(docker run --rm --entrypoint /usr/bin/krakend "$IMAGE" version 2>&1 \
        | awk '/^KrakenD Version:/ { print $3 }')"
echo "reported version: $ver"
case "$ver" in
  ""|undefined|0.0.0) echo "version not stamped: '$ver'"; exit 1 ;;
esac
if [ -n "$WANT" ] && [ "$ver" != "$WANT" ]; then
  echo "version mismatch: image reports '$ver', expected '$WANT'"; exit 1
fi

# The baked config must survive `krakend check` -- a default config that boots but
# does not validate would break any chart that lints before applying.
docker run --rm --entrypoint /usr/bin/krakend "$IMAGE" check -c /etc/krakend/krakend.json 2>&1 \
  | grep -q 'Syntax OK' || { echo "baked default config failed krakend check"; exit 1; }
echo "baked default config passes krakend check"

# must run as the nonroot krakend user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user)"
