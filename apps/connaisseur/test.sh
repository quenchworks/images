#!/usr/bin/env bash
# Smoke test for a built Connaisseur image. Usage: test.sh <image-ref> [version]
# Serves HTTPS with a throwaway certificate and two static validators, then sends real
# AdmissionReviews to /mutate: one image must be admitted, the other denied.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-connaisseur-smoke-$$"
# Mounted dirs live beside this script: Docker Desktop cannot mount /tmp here.
D="$(mktemp -d -p "$(cd "$(dirname "$0")" && pwd)")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$D"; }
trap cleanup EXIT
mkdir -p "$D/certs" "$D/config" "$D/alerts"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=connaisseur \
  -keyout "$D/certs/tls.key" -out "$D/certs/tls.crt" 2>/dev/null
cat > "$D/config/config.yaml" <<'YAML'
validators:
  - name: allow
    type: static
    approve: true
  - name: deny
    type: static
    approve: false
policy:
  - pattern: "*:*"
    validator: deny
  - pattern: "docker.io/library/allowed:*"
    validator: allow
YAML
echo "{}" > "$D/alerts/config.yaml"   # an empty file fails to parse (EOF)
chmod -R a+rX "$D"

# CACHE_EXPIRY_SECONDS=0: no Redis cache (otherwise it needs /app/redis-certs and a Redis)
docker run -d --name "$NAME" -p 127.0.0.1:5000:5000 -e CACHE_EXPIRY_SECONDS=0 \
  -v "$D/certs:/app/certs:ro" -v "$D/config:/app/config:ro" -v "$D/alerts:/app/alerts:ro" \
  "$IMAGE" >/dev/null
for i in $(seq 1 30); do
  curl -fsSk https://127.0.0.1:5000/health >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { echo "connaisseur did not come up"; docker logs "$NAME"; exit 1; }
  sleep 1
done

review() {
  cat <<JSON
{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{
 "uid":"$2","kind":{"group":"","version":"v1","kind":"Pod"},
 "resource":{"group":"","version":"v1","resource":"pods"},"namespace":"default","operation":"CREATE",
 "userInfo":{"username":"smoke"},
 "object":{"apiVersion":"v1","kind":"Pod","metadata":{"name":"p","namespace":"default"},
  "spec":{"containers":[{"name":"c","image":"$1"}]}}}}
JSON
}
for case in "allowed:1.0 true" "denied:1.0 false"; do
  set -- $case
  out="$(review "docker.io/library/$1" "uid-$1" | curl -fsSk -H 'Content-Type: application/json' \
    --data-binary @- https://127.0.0.1:5000/mutate)"
  grep -q "\"allowed\":$2" <<<"$out" || { echo "$1: expected allowed=$2, got: $out"; docker logs "$NAME"; exit 1; }
  echo "$1 -> allowed=$2"
done

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (user $user)"
