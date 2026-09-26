#!/usr/bin/env bash
# Smoke test for a built Fulcio image. Usage: test.sh <image-ref> [version]
# Starts Fulcio with its ephemeral test CA and default issuer config, and
# requires the v2 API to serve a trust bundle with a CA certificate and the
# issuer configuration.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-fulcio-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /usr/bin/fulcio "$IMAGE" version)"
grep -q "GitVersion: *v${WANT}" <<<"$ver" || { echo "version not v${WANT:-?}:"; echo "$ver"; exit 1; }

docker run -d --name "$NAME" -p 127.0.0.1:18080:8080 "$IMAGE" --ca ephemeralca >/dev/null
for i in $(seq 1 30); do
  bundle="$(curl -fsS http://127.0.0.1:18080/api/v2/trustBundle 2>/dev/null)" && break
  [ "$i" = 30 ] && { echo "server never answered"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
  sleep 1
done
grep -q "BEGIN CERTIFICATE" <<<"$bundle" || { echo "trust bundle has no certificate: $bundle"; exit 1; }
echo "  trust bundle served"
conf="$(curl -fsS http://127.0.0.1:18080/api/v2/configuration)"
grep -q '"issuers"' <<<"$conf" || { echo "no issuer configuration: $conf"; exit 1; }
echo "  issuer configuration served"

echo "smoke test passed (fulcio ${WANT:-?}: trust bundle, configuration; nonroot user: $user)"
