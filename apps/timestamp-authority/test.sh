#!/usr/bin/env bash
# Smoke test for a built Sigstore Timestamp Authority image. Usage: test.sh <image-ref> [version]
# Starts the server with its in-memory signer, fetches the certificate chain,
# asks for an RFC 3161 timestamp over JSON, and has openssl parse the reply.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-tsa-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /usr/bin/timestamp-server "$IMAGE" version)"
grep -q "GitVersion: *v${WANT}" <<<"$ver" || { echo "version not v${WANT:-?}:"; echo "$ver"; exit 1; }

docker run -d --name "$NAME" -p 127.0.0.1:13000:3000 "$IMAGE" --disable-ntp-monitoring >/dev/null
for i in $(seq 1 30); do
  curl -fsS -o "$WORK/chain.pem" http://127.0.0.1:13000/api/v1/timestamp/certchain 2>/dev/null && break
  [ "$i" = 30 ] && { echo "server never answered"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
  sleep 1
done
grep -q "BEGIN CERTIFICATE" "$WORK/chain.pem" || { echo "no certificate chain"; exit 1; }
echo "  certificate chain served"

hash="$(printf 'quench' | openssl dgst -sha256 -binary | base64)"
code="$(curl -sS -o "$WORK/reply.tsr" -w '%{http_code}' -H 'Content-Type: application/json' \
  -d "{\"artifactHash\":\"$hash\",\"hashAlgorithm\":\"sha256\",\"certificates\":true}" \
  http://127.0.0.1:13000/api/v1/timestamp)"
echo "  timestamp request: HTTP $code"
case "$code" in 200|201) ;; *) echo "timestamp not issued"; docker logs "$NAME" 2>&1 | tail -20; exit 1 ;; esac
status="$(openssl ts -reply -in "$WORK/reply.tsr" -text | grep -m1 '^Status:')"
echo "  $status"
grep -q "Granted" <<<"$status" || { echo "reply not granted"; exit 1; }

echo "smoke test passed (timestamp-authority ${WANT:-?}: cert chain, RFC 3161 timestamp granted; nonroot user: $user)"
