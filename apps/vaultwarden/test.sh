#!/usr/bin/env bash
# Smoke test for a built Vaultwarden image. Usage: test.sh <image-ref> [version]
# Vaultwarden runs on a read-only root with /data on a volume. A Python client does what a Bitwarden
# client does: derive the master password hash (PBKDF2), register, log in with the password grant,
# store a cipher and read it back from /api/sync, also after a restart. A wrong password must be
# refused and the web vault must be served.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="vaultwarden-smoke-$$"
URL=http://127.0.0.1:18081
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
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

client() { python3 - "$URL" "$@" <<'PY'
import base64, hashlib, json, sys, urllib.parse, urllib.request, uuid
url, action = sys.argv[1], sys.argv[2]
email, pw = "smoke@quench.test", sys.argv[3] if len(sys.argv) > 3 else "smoke-master-pass-2026"
def mhash(p):
    key = hashlib.pbkdf2_hmac("sha256", p.encode(), email.encode(), 600000)
    return base64.b64encode(hashlib.pbkdf2_hmac("sha256", key, p.encode(), 1)).decode()
def req(path, data=None, form=None, tok=None):
    h = {}
    body = None
    if data is not None: body, h["Content-Type"] = json.dumps(data).encode(), "application/json"
    if form is not None: body, h["Content-Type"] = urllib.parse.urlencode(form).encode(), "application/x-www-form-urlencoded"
    if tok: h["Authorization"] = "Bearer " + tok
    h["Accept"] = "application/json"
    r = urllib.request.Request(url + path, data=body, headers=h, method="POST" if body is not None else "GET")
    try:
        with urllib.request.urlopen(r, timeout=30) as resp: return resp.status, resp.read().decode()
    except urllib.error.HTTPError as e: return e.code, e.read().decode()
enc = "2.c21va2VpdjEyMzQ1Njc4OQ==|c21va2VjdA==|c21va2VtYWNtYWNtYWNtYWNtYWNtYWNtYWNtYWM="
if action == "register":
    body = {"email": email, "name": "Smoke", "masterPasswordHash": mhash(pw), "masterPasswordHint": None,
            "key": enc, "kdf": 0, "kdfIterations": 600000,
            "keys": {"publicKey": "c21va2UtcHVibGljLWtleQ==", "encryptedPrivateKey": enc}}
    # two steps since 1.35: with mail off the verification call returns the token itself
    code, out = req("/identity/accounts/register/send-verification-email", {"email": email, "name": "Smoke"})
    if code != 200: print("verification", code, out[:300]); sys.exit(1)
    body["emailVerificationToken"] = json.loads(out)
    code, out = req("/identity/accounts/register/finish", body)
    print(code, out[:300]); sys.exit(0 if code in (200, 204) else 1)
form = {"grant_type": "password", "username": email, "password": mhash(pw), "scope": "api offline_access",
        "client_id": "web", "deviceType": "9", "deviceIdentifier": str(uuid.uuid4()), "deviceName": "smoke"}
code, out = req("/identity/connect/token", form=form)
if action == "badlogin": print(code); sys.exit(0 if code in (400, 401) else 1)
if code != 200: print("login failed", code, out[:300]); sys.exit(1)
tok = json.loads(out)["access_token"]
if action == "store":
    seg = tok.split(".")[1]; sub = json.loads(base64.urlsafe_b64decode(seg + "=" * (-len(seg) % 4)))["sub"]
    code, out = req("/api/ciphers", {"type": 2, "name": enc, "notes": None, "secureNote": {"type": 0}, "favorite": False, "encryptedFor": sub}, tok=tok)
    print(code); sys.exit(0 if code == 200 else 1)
code, out = req("/api/sync", tok=tok)
n = len(json.loads(out).get("ciphers") or json.loads(out).get("Ciphers") or [])
print(n); sys.exit(0 if code == 200 and n == 1 else 1)
PY
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
