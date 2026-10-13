#!/usr/bin/env bash
# Smoke test for a built satosa image. Usage: test.sh <image-ref> [version]
# Boots the proxy on a read-only root with a minimal config (SAML2 backend exposing its
# metadata, ping frontend), hits both endpoints, then signs the proxy metadata with the
# satosa-saml-metadata CLI (pysaml2 -> xmlsec1) and verifies the signature with xmlsec1.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="satosa-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT
trap 'echo "failed at line $LINENO"; docker logs "$NAME" 2>&1 | tail -30' ERR

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint /opt/satosa/venv/bin/python "$IMAGE" -c "import importlib.metadata as m; print(m.version('satosa'))")"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "version mismatch: $ver"; exit 1; }
docker run --rm --entrypoint /usr/bin/xmlsec1 "$IMAGE" --version

B=http://127.0.0.1:18080
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj /CN=satosa-smoke \
  -keyout "$WORK/tls.key" -out "$WORK/tls.crt" 2>/dev/null
cat > "$WORK/proxy_conf.yaml" <<EOF
BASE: $B
COOKIE_STATE_NAME: SATOSA_STATE
STATE_ENCRYPTION_KEY: smoke-state-key-$$-0123456789
INTERNAL_ATTRIBUTES:
  attributes:
    mail:
      saml: [mail]
BACKEND_MODULES:
  - module: satosa.backends.saml2.SAMLBackend
    name: Saml2
    config:
      entityid_endpoint: true
      sp_config:
        entityid: $B/Saml2/metadata.xml
        key_file: /etc/satosa/tls.key
        cert_file: /etc/satosa/tls.crt
        metadata:
          local: []
        service:
          sp:
            endpoints:
              assertion_consumer_service:
                - [$B/Saml2/acs/post, "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"]
FRONTEND_MODULES:
  - module: satosa.frontends.ping.PingFrontend
    name: ping
    config: {}
EOF
mkdir "$WORK/out"
chmod 0644 "$WORK"/tls.* "$WORK/proxy_conf.yaml"; chmod 0777 "$WORK/out"
MOUNTS=(-v "$WORK/proxy_conf.yaml:/etc/satosa/proxy_conf.yaml:ro" -v "$WORK/tls.key:/etc/satosa/tls.key:ro" -v "$WORK/tls.crt:/etc/satosa/tls.crt:ro")

docker run -d --name "$NAME" --read-only --tmpfs /tmp "${MOUNTS[@]}" -p 127.0.0.1:18080:8080 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 60); do curl -fsS -o /dev/null "$B/ping" 2>/dev/null && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] || { echo "satosa never answered /ping"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
md="$(curl -fsS "$B/Saml2/metadata.xml")"
grep -q 'entityID="'"$B"'/Saml2/metadata.xml"' <<<"$md" || { echo "no SP metadata: $(head -c 300 <<<"$md")"; exit 1; }
grep -q 'X509Certificate' <<<"$md" || { echo "metadata carries no certificate"; exit 1; }

# signed metadata through pysaml2 + xmlsec1, then an independent xmlsec1 verify
docker run --rm --read-only --tmpfs /tmp "${MOUNTS[@]}" -v "$WORK/out:/out" --entrypoint /opt/satosa/venv/bin/satosa-saml-metadata \
  "$IMAGE" /etc/satosa/proxy_conf.yaml /etc/satosa/tls.key /etc/satosa/tls.crt --dir /out
grep -q 'SignatureValue' "$WORK/out/backend.xml" || { echo "backend.xml is not signed"; exit 1; }
docker run --rm --read-only --tmpfs /tmp "${MOUNTS[@]}" -v "$WORK/out:/out:ro" --entrypoint /usr/bin/xmlsec1 "$IMAGE" \
  --verify --pubkey-cert-pem /etc/satosa/tls.crt --id-attr:ID urn:oasis:names:tc:SAML:2.0:metadata:EntitiesDescriptor /out/backend.xml

logs="$(docker logs "$NAME" 2>&1)"
if grep -E 'Traceback|CRITICAL' <<<"$logs"; then echo "satosa logged errors"; exit 1; fi
echo "smoke test passed (satosa ${WANT:-$ver}, uid $user, /ping + SP metadata served, metadata signed and verified by xmlsec1)"
