#!/usr/bin/env bash
# Smoke test for a built Vaultwarden image. Usage: test.sh <image-ref> [version]
# Vaultwarden runs on a read-only root with /data on a volume. A shell client (curl, jq, openssl) does what a Bitwarden
# client does: derive the master password hash (PBKDF2), register, log in with the password grant,
# store a cipher and read it back from /api/sync, also after a restart. A wrong password must be
# refused and the web vault must be served.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="vaultwarden-smoke-$$"
URL=http://127.0.0.1:18081
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; rm -rf -- "${T:-}"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm --entrypoint /usr/bin/vaultwarden "$IMAGE" --version 2>&1)"
echo "$v"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

docker volume create "$NAME" >/dev/null
start() {
  docker run -d --name "$NAME" --read-only --tmpfs /tmp:uid=1001,gid=1001 -v "$NAME:/data" -p 127.0.0.1:18081:8080 "$IMAGE" >/dev/null
  for i in $(seq 1 60); do curl -fsS "$URL/alive" >/dev/null 2>&1 && return 0
    [ "$i" = 60 ] && { echo "/alive never answered"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }; sleep 1; done
}
start
curl -fsS "$URL/" | grep -qi '<html' || { echo "web vault not served"; exit 1; }
echo "  /alive ok, web vault served"

EMAIL=smoke@quench.test
ENC="2.c21va2VpdjEyMzQ1Njc4OQ==|c21va2VjdA==|c21va2VtYWNtYWNtYWNtYWNtYWNtYWNtYWNtYWM="
T="$(mktemp -d)"
# Bitwarden's master password hash: PBKDF2-SHA256(PBKDF2-SHA256(password, email, 600000), password, 1)
mhash() { local k; k="$(openssl kdf -keylen 32 -kdfopt digest:SHA256 -kdfopt pass:"$1" -kdfopt salt:"$EMAIL" -kdfopt iter:600000 PBKDF2 | tr -d ':')"; openssl kdf -keylen 32 -kdfopt digest:SHA256 -kdfopt hexpass:"$k" -kdfopt salt:"$1" -kdfopt iter:1 -binary PBKDF2 | base64 -w0; }
# http PATH [curl args]: body to $T/out, prints the status code
http() { local p="$1"; shift; curl -s -m 30 -o "$T/out" -w '%{http_code}' -H 'Accept: application/json' "$@" "$URL$p"; }
json() { http "$1" -H 'Content-Type: application/json' -d "$2" "${@:3}"; }
# client ACTION [password]: what a Bitwarden client does, one step per action
client() {
  local action="$1" pw="${2:-smoke-master-pass-2026}" code tok sub n body
  if [ "$action" = register ]; then
    # two steps since 1.35: with mail off the verification call returns the token itself
    code="$(json /identity/accounts/register/send-verification-email "$(jq -nc --arg e "$EMAIL" '{email: $e, name: "Smoke"}')")"
    [ "$code" = 200 ] || { echo "verification $code $(head -c 300 "$T/out")"; return 1; }
    body="$(jq -nc --arg e "$EMAIL" --arg h "$(mhash "$pw")" --arg k "$ENC" --slurpfile t "$T/out" \
      '{email: $e, name: "Smoke", masterPasswordHash: $h, masterPasswordHint: null, key: $k, kdf: 0, kdfIterations: 600000,
        keys: {publicKey: "c21va2UtcHVibGljLWtleQ==", encryptedPrivateKey: $k}, emailVerificationToken: $t[0]}')"
    code="$(json /identity/accounts/register/finish "$body")"
    echo "$code $(head -c 300 "$T/out")"; [ "$code" = 200 ] || [ "$code" = 204 ]; return
  fi
  code="$(http /identity/connect/token --data-urlencode grant_type=password --data-urlencode "username=$EMAIL" \
    --data-urlencode "password=$(mhash "$pw")" --data-urlencode 'scope=api offline_access' --data-urlencode client_id=web \
    --data-urlencode deviceType=9 --data-urlencode "deviceIdentifier=$(cat /proc/sys/kernel/random/uuid)" --data-urlencode deviceName=smoke)"
  if [ "$action" = badlogin ]; then echo "$code"; [ "$code" = 400 ] || [ "$code" = 401 ]; return; fi
  [ "$code" = 200 ] || { echo "login failed $code $(head -c 300 "$T/out")"; return 1; }
  tok="$(jq -er .access_token "$T/out")"
  if [ "$action" = store ]; then
    sub="$(cut -d. -f2 <<<"$tok" | tr '_-' '/+' | awk '{l=length($0)%4; if(l) $0=$0 substr("===",1,4-l); print}' | base64 -d | jq -er .sub)"
    code="$(json /api/ciphers "$(jq -nc --arg n "$ENC" --arg s "$sub" '{type: 2, name: $n, notes: null, secureNote: {type: 0}, favorite: false, encryptedFor: $s}')" -H "Authorization: Bearer $tok")"
    echo "$code"; [ "$code" = 200 ]; return
  fi
  code="$(http /api/sync -H "Authorization: Bearer $tok")"
  n="$(jq '(.ciphers // .Ciphers // []) | length' "$T/out")"
  echo "$n"; [ "$code" = 200 ] && [ "$n" = 1 ]
}
client register || { echo "register failed"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
code="$(client badlogin wrong-master-pass)" || { echo "a wrong password got HTTP $code"; exit 1; }
client store >/dev/null || { echo "storing a cipher failed"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
client sync >/dev/null || { echo "cipher not in /api/sync"; exit 1; }
echo "  registered, wrong password refused ($code), cipher stored and synced"
docker rm -f "$NAME" >/dev/null
start
client sync >/dev/null || { echo "cipher lost across a restart"; exit 1; }
echo "  cipher kept across a restart"
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|panicked'; then echo "crashed or wrote to the root"; docker logs "$NAME" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (vaultwarden ${WANT:-?}: Bitwarden register/login, cipher stored and synced across a restart on /data, read-only root; user $user)"
