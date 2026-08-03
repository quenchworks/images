#!/usr/bin/env bash
# Smoke test for a built HashiCorp Vault image. Usage: test.sh <image-ref>
#
# This is a REAL functional check, not a TCP accept -- a TCP-only probe passes on a
# process that is listening but dead, and Vault in particular listens happily while
# sealed and unable to serve a single secret.
#
#   1) DEFAULT server (the baked /etc/vault/vault.hcl, `file` storage): assert the API
#      answers 501 (uninitialized), then drive the real operator flow through the
#      in-image CLI -- `operator init` -> `operator unseal` -> health 200 -> `status`
#      reports Sealed=false -> enable kv-v2 -> put/get a secret and compare the value.
#   2) DEV mode (what the chart's CI gate runs): auto-unsealed, health 200, kv roundtrip.
#
# The image is SHELL-FREE, so every exec passes an argv directly (no `sh -c`) and env
# comes from `docker exec -e`.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
SRV="quench-vault-srv-$$"
DEV="quench-vault-dev-$$"

cleanup() { docker rm -f "$SRV" "$DEV" >/dev/null 2>&1 || true; }
trap cleanup EXIT

command -v jq >/dev/null || { echo "jq is required"; exit 1; }

# curl the health endpoint, print the status code.
health() { curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$1/v1/sys/health" || true; }

# wait_health <port> <expected-code...>
wait_health() {
  local port="$1"; shift
  local code i want
  for i in $(seq 1 40); do
    code="$(health "$port")"
    for want in "$@"; do [ "$code" = "$want" ] && { echo "$code"; return 0; }; done
    sleep 1
  done
  echo "health on :$port never reached [$*] (last=$code)" >&2
  return 1
}

# --- must run as the nonroot vault user (uid 1001) ---
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# --- the image must be bootable with NO arguments (baked default config) ---
cmd="$(docker inspect "$IMAGE" --format '{{join .Config.Cmd " "}}')"
[ "$cmd" = "server -config=/etc/vault/vault.hcl" ] || { echo "unexpected Cmd: '$cmd'"; exit 1; }

############################################################
# 1) DEFAULT server: init -> unseal -> kv roundtrip
############################################################
echo "== default server: starting $IMAGE (read-only rootfs + tmpfs, no args) =="
docker run -d --name "$SRV" \
  --read-only \
  --tmpfs /vault/data:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -p 18200:8200 \
  "$IMAGE" >/dev/null

echo "waiting for the API to report 501 (up but UNINITIALIZED -- expected)"
code="$(wait_health 18200 501)" || { docker logs "$SRV"; exit 1; }
echo "health $code OK (listening, uninitialized)"

echo "vault operator init (1 share, threshold 1)"
init="$(docker exec "$SRV" vault operator init -key-shares=1 -key-threshold=1 -format=json)"
key="$(jq -r '.unseal_keys_b64[0]' <<<"$init")"
root="$(jq -r '.root_token' <<<"$init")"
[ -n "$key" ] && [ "$key" != null ] || { echo "no unseal key in init output"; exit 1; }
[ -n "$root" ] && [ "$root" != null ] || { echo "no root token in init output"; exit 1; }

echo "vault operator unseal"
docker exec "$SRV" vault operator unseal "$key" >/dev/null

code="$(wait_health 18200 200)" || { docker logs "$SRV"; exit 1; }
echo "health $code OK (initialized + unsealed)"

# `vault status` exits 2 while sealed, 0 when unsealed -- so a plain run is itself an
# assertion, and we check the field too.
docker exec "$SRV" vault status | grep -E '^Sealed +false$' \
  || { echo "vault status does not report Sealed=false"; docker exec "$SRV" vault status; exit 1; }

echo "kv-v2 put/get roundtrip through the API"
docker exec -e VAULT_TOKEN="$root" "$SRV" vault secrets enable -path=secret kv-v2 >/dev/null
docker exec -e VAULT_TOKEN="$root" "$SRV" vault kv put secret/quench value=hello >/dev/null
got="$(docker exec -e VAULT_TOKEN="$root" "$SRV" vault kv get -field=value secret/quench)"
[ "$got" = "hello" ] || { echo "kv roundtrip failed: got '$got'"; docker logs "$SRV"; exit 1; }
echo "kv roundtrip OK (value='$got')"

echo "version: $(docker exec "$SRV" vault version)"

# --- the web UI must really be compiled in (-tags ui) ---
# Do NOT assert on the HTTP status: a Vault built WITHOUT the ui tag also answers /ui/
# with 200, serving a ~1.4KB stub that reads "Vault UI is not available in this binary."
# (Measured against the previous QuenchWorks image, which returned exactly that.) The
# real console is a ~1MB Ember index.html referencing hashed bundles under /ui/assets/,
# so assert on content and then fetch an asset to prove it is actually served.
echo "web UI: GET /ui/"
curl -sS -o /tmp/vault-ui-$$.html "http://127.0.0.1:18200/ui/"
if grep -qi 'not available in this binary' "/tmp/vault-ui-$$.html"; then
  echo "UI stub served -- the binary was built without -tags ui"; exit 1
fi
asset="$(grep -oE '/ui/assets/[A-Za-z0-9._-]+\.js' "/tmp/vault-ui-$$.html" | head -1)"
[ -n "$asset" ] || { echo "no Ember bundle referenced in /ui/"; exit 1; }
acode="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:18200$asset")"
[ "$acode" = "200" ] || { echo "UI asset $asset did not serve (http $acode)"; exit 1; }
echo "web UI OK ($(wc -c <"/tmp/vault-ui-$$.html") bytes, asset $asset serves 200)"
rm -f "/tmp/vault-ui-$$.html"

############################################################
# 2) DEV mode (the chart's CI gate path)
############################################################
echo "== dev mode: auto-unsealed in-memory node =="
docker run -d --name "$DEV" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777 \
  -p 18300:8200 \
  -e VAULT_TOKEN=root \
  --entrypoint /usr/bin/vault \
  "$IMAGE" server -dev -dev-listen-address=0.0.0.0:8200 -dev-root-token-id=root -dev-no-store-token >/dev/null

code="$(wait_health 18300 200)" || { docker logs "$DEV"; exit 1; }
echo "dev health $code OK (auto-unsealed)"

# dev mode pre-mounts kv-v2 at secret/; VAULT_TOKEN comes from the container env, which
# is exactly how the chart's gate authenticates (no shell to export it in).
docker exec "$DEV" vault kv put secret/quench value=devhello >/dev/null
got="$(docker exec "$DEV" vault kv get -field=value secret/quench)"
[ "$got" = "devhello" ] || { echo "dev kv roundtrip failed: got '$got'"; docker logs "$DEV"; exit 1; }
echo "dev kv roundtrip OK (value='$got')"

echo "smoke test passed (nonroot $user, default node init+unseal+kv OK, dev node kv OK)"
