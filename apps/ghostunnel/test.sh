#!/usr/bin/env bash
# Smoke test for a built ghostunnel image. Usage: test.sh <image-ref> [version]
# A test CA signs a server and a client certificate. A ghostunnel server wraps its own status
# endpoint in mTLS; a ghostunnel client exposes it as plaintext. Fetching the status through
# the client proves both modes and the mutual handshake; a TLS client without a certificate
# must be refused by the server.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="gt-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$TAG-srv" "$TAG-cli" "$TAG-cp" >/dev/null 2>&1 || true; docker volume rm "$TAG" >/dev/null 2>&1 || true; docker network rm "$TAG" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$v"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

cd "$WORK"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj /CN=smoke-ca \
  -keyout ca.key -out ca.crt 2>/dev/null
for n in server client; do
  openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=$n" -keyout $n.key -out $n.csr 2>/dev/null
  printf 'subjectAltName=DNS:%s\nextendedKeyUsage=serverAuth,clientAuth\n' "$TAG-srv" > $n.ext
  openssl x509 -req -in $n.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 1 -extfile $n.ext -out $n.crt 2>/dev/null
done
chmod a+r ./*; chmod a+rx .
cd - >/dev/null

# certificates go in a volume, not a bind mount: ghostunnel's landlock sandbox gets EACCES on
# Docker Desktop's virtiofs bind mounts (upstream's binary too), and a volume behaves like the
# Secret volume a pod mounts
docker volume create "$TAG" >/dev/null
docker create --name "$TAG-cp" -v "$TAG:/certs" "$IMAGE" >/dev/null
docker cp "$WORK/." "$TAG-cp:/certs/" >/dev/null
docker rm "$TAG-cp" >/dev/null
docker network create "$TAG" >/dev/null
docker run -d --name "$TAG-srv" --network "$TAG" --read-only -v "$TAG:/certs:ro" -p 127.0.0.1:18443:8443 "$IMAGE" \
  server --listen 0.0.0.0:8443 --target 127.0.0.1:6060 --status http://127.0.0.1:6060 \
  --cert /certs/server.crt --key /certs/server.key --cacert /certs/ca.crt --allow-cn client >/dev/null
docker run -d --name "$TAG-cli" --network "$TAG" --read-only -v "$TAG:/certs:ro" -p 127.0.0.1:18080:8080 "$IMAGE" \
  client --listen 0.0.0.0:8080 --unsafe-listen --target "$TAG-srv:8443" \
  --cert /certs/client.crt --key /certs/client.key --cacert /certs/ca.crt >/dev/null

out=""
for _ in $(seq 1 30); do
  out="$(curl -sS --max-time 5 http://127.0.0.1:18080/_status 2>/dev/null || true)"
  grep -q '"ok"' <<<"$out" && break
  sleep 1
done
grep -q '"ok"' <<<"$out" || { echo "status not reachable through the tunnel: $out"; docker logs "$TAG-srv" 2>&1 | tail -15; docker logs "$TAG-cli" 2>&1 | tail -15; exit 1; }
echo "through client -> mTLS -> server: $out"
if curl -sS --max-time 5 --cacert "$WORK/ca.crt" --resolve "$TAG-srv:18443:127.0.0.1" "https://$TAG-srv:18443/_status" >/dev/null 2>&1; then
  echo "server accepted a client without a certificate"; exit 1
fi
echo "smoke test passed: mTLS server and client modes, certificate-less client refused, user 1001"
