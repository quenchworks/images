#!/usr/bin/env bash
# Smoke test for a built Rekor v2 (rekor-tiles) image. Usage: test.sh <image-ref> [version]
# Starts the server with a throwaway ed25519 checkpoint key, requires /healthz
# to report SERVING, then submits an ECDSA-signed hashedrekord entry and
# requires it to be sequenced at index 0 with an inclusion proof.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-rekor-tiles-smoke-$$"
WORK="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /usr/bin/rekor-server "$IMAGE" version)"
grep -q "GitVersion: *v${WANT}" <<<"$ver" || { echo "version not v${WANT:-?}:"; echo "$ver"; exit 1; }

openssl genpkey -algorithm ed25519 -out "$WORK/signer.pem" 2>/dev/null
chmod 0644 "$WORK/signer.pem"
docker run -d --name "$NAME" -p 127.0.0.1:13000:3000 -v "$WORK/signer.pem:/pki/signer.pem:ro" "$IMAGE" \
  --hostname=rekor.smoke --signer-filepath=/pki/signer.pem --checkpoint-interval=1s >/dev/null
for i in $(seq 1 30); do
  health="$(curl -fsS http://127.0.0.1:13000/healthz 2>/dev/null)" && break
  [ "$i" = 30 ] && { echo "server never answered"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
  sleep 1
done
grep -qF '"status":"SERVING"' <<<"$health" || { echo "not serving: $health"; exit 1; }
echo "  healthz SERVING"

echo "quench smoke artifact" > "$WORK/artifact"
openssl ecparam -name prime256v1 -genkey -noout -out "$WORK/ec.pem"
openssl dgst -sha256 -sign "$WORK/ec.pem" -out "$WORK/artifact.sig" "$WORK/artifact"
digest="$(openssl dgst -sha256 -binary "$WORK/artifact" | base64 -w0)"
sig="$(base64 -w0 "$WORK/artifact.sig")"
pub="$(openssl ec -in "$WORK/ec.pem" -pubout -outform DER 2>/dev/null | base64 -w0)"
printf '{"hashedRekordRequestV002":{"digest":"%s","signature":{"content":"%s","verifier":{"publicKey":{"rawBytes":"%s"},"keyDetails":"PKIX_ECDSA_P256_SHA_256"}}}}' \
  "$digest" "$sig" "$pub" > "$WORK/req.json"
resp="$(curl -sS -w '\n%{http_code}' -X POST -H 'Content-Type: application/json' \
  --data @"$WORK/req.json" http://127.0.0.1:13000/api/v2/log/entries)"
code="${resp##*$'\n'}"
[ "$code" = "201" ] || { echo "entry rejected: HTTP $code: ${resp%$'\n'*}"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
grep -qF '"logIndex":"0"' <<<"$resp" && grep -qF '"inclusionProof"' <<<"$resp" \
  || { echo "no index or inclusion proof: $resp"; exit 1; }
echo "  hashedrekord entry sequenced at index 0 with an inclusion proof"

echo "smoke test passed (rekor-tiles ${WANT:-?}: healthz, signed entry; nonroot user: $user)"
