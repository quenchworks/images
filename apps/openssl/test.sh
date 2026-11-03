#!/usr/bin/env bash
# Smoke test for a built openssl image. Usage: test.sh <image-ref> [version]
# Real work: builds a CA and a leaf, exports a PKCS12 keystore from the PEM pair the way
# the chart helper does, and reads it back, all inside the image as uid 1001.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"

ver="$(docker run --rm "$IMAGE" version)"
echo "  $ver"
[ -z "$WANT" ] || grep -q "OpenSSL $WANT " <<<"$ver" || { echo "expected OpenSSL $WANT"; exit 1; }

out="$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c '
  set -eu; cd /tmp
  openssl req -x509 -newkey rsa:2048 -nodes -keyout ca.key -out ca.crt -subj /CN=ca -days 1 2>/dev/null
  openssl req -newkey rsa:2048 -nodes -keyout tls.key -out tls.csr -subj /CN=localhost 2>/dev/null
  openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out tls.crt -days 1 2>/dev/null
  openssl pkcs12 -export -inkey tls.key -in tls.crt -certfile ca.crt -name tls -passout pass:changeit -out ks.p12
  openssl pkcs12 -in ks.p12 -passin pass:changeit -nokeys | grep -c "BEGIN CERTIFICATE"
')"
[ "$out" = 2 ] || { echo "PKCS12 round trip failed: $out"; exit 1; }
echo "  PKCS12 export + read back: leaf and CA present"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (OpenSSL ${WANT:-?}, nonroot $user)"
