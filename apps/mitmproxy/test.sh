#!/usr/bin/env bash
# Smoke test for a built mitmproxy image. Usage: test.sh <image-ref> [version]
#
# An open port proves nothing, so this test intercepts TLS:
#   1. mitmdump --version reports the version.
#   2. Two containers from the image, both on a READ-ONLY rootfs as uid 1001 (tmpfs home for
#      the generated CA): "web" runs mitmweb (tornado UI on 8081) with a TLS reverse listener
#      on 8443 in front of it; "proxy" runs mitmdump as a regular proxy on 8080. (mitmproxy
#      refuses to forward to its own listener, hence two.)
#   3. curl goes through the proxy to https://web:8443/ and verifies the certificate against
#      the proxy's generated CA: the proxy intercepted TLS (forged a leaf with
#      cryptography/pyOpenSSL), opened TLS upstream to web's reverse listener, which forwarded
#      to the mitmweb UI. The page coming back proves every hop and tornado.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-mitmproxy-smoke-$$"
WORK="$(mktemp -d)"
PY=/opt/mitmproxy/venv/bin/python
free_port() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }

cleanup() { docker rm -f "$NAME" "$NAME-web" >/dev/null 2>&1 || true; docker network rm "$NAME-net" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

ver_out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$ver_out"
grep -q '^Mitmproxy: ' <<<"$ver_out" || { echo "no version banner"; exit 1; }
if [ -n "${2:-}" ]; then
  grep -qx "Mitmproxy: ${2}" <<<"$ver_out" || { echo "expected version $2"; exit 1; }
fi

NET="$NAME-net"
docker network create "$NET" >/dev/null
RO="--read-only --tmpfs /tmp --tmpfs /home/mitmproxy:uid=1001"
docker run -d --name "$NAME-web" --network "$NET" $RO --entrypoint /usr/bin/mitmweb "$IMAGE" \
  --mode "reverse:http://127.0.0.1:8081@8443" --set web_open_browser=false --set web_password=quench-smoke >/dev/null
PROXY="$(free_port)"
docker run -d --name "$NAME" --network "$NET" $RO -p "127.0.0.1:${PROXY}:8080" "$IMAGE" \
  --mode regular@8080 --set ssl_insecure=true >/dev/null

ca="$WORK/ca.pem"
for i in $(seq 1 60); do
  if docker exec "$NAME" "$PY" -c "print(open('/home/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem').read(), end='')" > "$ca" 2>/dev/null \
     && grep -q 'BEGIN CERTIFICATE' "$ca"; then
    body="$(curl -sS -m 10 --cacert "$ca" -x "http://127.0.0.1:${PROXY}" https://$NAME-web:8443/ 2>/dev/null || true)"
    grep -qi 'mitmproxy' <<<"$body" && break
  fi
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "container died:"; docker logs "$NAME"; exit 1; }
  [ "$i" = 60 ] && { echo "no intercepted HTTPS response"; docker logs "$NAME" | tail -30; docker logs "$NAME-web" | tail -30; exit 1; }
  sleep 1
done
echo "HTTPS through the proxy, verified against mitmproxy's CA: $(grep -oi '<title>[^<]*' <<<"$body" | head -1)"

# the interception really happened: the leaf curl saw is issued by mitmproxy's CA
issuer="$(curl -sS -m 10 -v --cacert "$ca" -x "http://127.0.0.1:${PROXY}" https://$NAME-web:8443/ -o /dev/null 2>&1 | grep -i 'issuer:' | head -1)"
grep -qi 'mitmproxy' <<<"$issuer" || { echo "the certificate was not issued by mitmproxy: $issuer"; exit 1; }
echo "leaf $issuer"

for c in "$NAME" "$NAME-web"; do
  if docker logs "$c" 2>&1 | grep -qiE 'read-only file system|permission denied|Traceback'; then
    echo "errors in the logs of $c:"; docker logs "$c"; exit 1
  fi
done

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only boot, TLS interception, mitmweb UI through the proxy)"
