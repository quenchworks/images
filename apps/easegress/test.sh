#!/usr/bin/env bash
# Smoke test for a built Easegress image. Usage: test.sh <image-ref> [version]
# Creates an HTTPServer and a Pipeline through the admin API, then sends a request
# through the new traffic port: the Pipeline's Proxy filter forwards it to the admin
# API itself, so a reply proves the data path, not only that the server started.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-easegress-smoke-$$"
ADMIN=http://127.0.0.1:12381/apis/v2

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ver="$(docker run --rm "$IMAGE" --version 2>&1 | head -5)"
echo "reported: $ver"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$ver" || { echo "expected v$WANT"; exit 1; }

echo "starting $IMAGE"
docker run -d --name "$NAME" -p 127.0.0.1:12381:2381 -p 127.0.0.1:10080:10080 "$IMAGE" >/dev/null
for i in $(seq 1 60); do
  curl -fsS "$ADMIN/healthz" >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "admin API never answered"; docker logs "$NAME"; exit 1; }
  sleep 1
done

create() {
  curl -fsS -X POST -H 'Content-Type: text/x-yaml' --data-binary @- "$ADMIN/objects" >/dev/null \
    || { echo "creating an object failed"; docker logs "$NAME" | tail -20; exit 1; }
}
create <<'YAML'
name: qw-pipeline
kind: Pipeline
flow:
  - filter: proxy
filters:
  - name: proxy
    kind: Proxy
    pools:
      - servers:
          - url: http://127.0.0.1:2381
YAML
create <<'YAML'
name: qw-server
kind: HTTPServer
port: 10080
rules:
  - paths:
      - pathPrefix: /apis
        backend: qw-pipeline
YAML

got=""
for i in $(seq 1 30); do
  got="$(curl -fsS http://127.0.0.1:10080/apis/v2/objects/qw-pipeline 2>/dev/null)" && break
  [ "$i" = 30 ] && { echo "traffic port 10080 never proxied"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
grep -q "qw-pipeline" <<<"$got" || { echo "proxied reply lacks the object: $got"; exit 1; }
echo "  HTTPServer :10080 -> Pipeline -> Proxy -> admin API round-trips"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# objects live in the embedded etcd under the home dir: they survive a restart
docker restart "$NAME" >/dev/null
for i in $(seq 1 60); do
  curl -fsS http://127.0.0.1:10080/apis/v2/healthz >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "objects did not come back after a restart"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done

echo "smoke test passed ($ver, nonroot user: $user, proxy data path, objects kept across restart)"
