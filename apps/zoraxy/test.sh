#!/usr/bin/env bash
# Smoke test for a built Zoraxy image. Usage: test.sh <image-ref> [version]
# Starts Zoraxy and drives the admin panel over plain HTTP the way the browser does: reads the
# CSRF token from the login page, registers the first admin, logs in and lists proxy rules.
# A POST without the token must be refused (gorilla/csrf 1.7.3 with the plaintext patch).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-zoraxy-smoke-$$"
WORK="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" -version)"
echo "zoraxy: $ver"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$ver" || { echo "expected $WANT"; exit 1; }

docker run -d --name "$NAME" -p 127.0.0.1::8000 "$IMAGE" >/dev/null
port="$(docker port "$NAME" 8000/tcp | head -1 | sed 's/.*://')"
B="http://127.0.0.1:$port"
J="$WORK/jar"
for i in $(seq 1 60); do
  curl -fsS -c "$J" -b "$J" "$B/login.html" -o "$WORK/login.html" 2>/dev/null && break
  [ "$i" = 60 ] && { echo "panel did not come up"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
tok() { sed -n 's/.*name="zoraxy.csrf.Token" content="\([^"]*\)".*/\1/p' "$1" | head -1; }
t="$(tok "$WORK/login.html")"
[ -n "$t" ] || { echo "no CSRF token on the login page"; exit 1; }

post() { curl -sS -c "$J" -b "$J" -H "Origin: $B" -H "Referer: $B/login.html" "$@"; }
code="$(post -o /dev/null -w '%{http_code}' -d 'username=admin&password=quench-pass-1' "$B/api/auth/register")"
[ "$code" = 403 ] || { echo "register without a CSRF token got HTTP $code, want 403"; exit 1; }
out="$(post -H "X-CSRF-Token: $t" -d 'username=admin&password=quench-pass-1' "$B/api/auth/register")"
echo "register: $out"
grep -q '"error"' <<<"$out" && { echo "register failed"; docker logs "$NAME" | tail -20; exit 1; }
out="$(post -H "X-CSRF-Token: $t" -d 'username=admin&password=quench-pass-1' "$B/api/auth/login")"
echo "login: $out"
grep -q '"error"' <<<"$out" && { echo "login failed"; exit 1; }
list="$(curl -fsS -c "$J" -b "$J" "$B/api/proxy/list?type=host")"
grep -q '^\[' <<<"$list" || { echo "proxy list is not a JSON array: $list"; exit 1; }
echo "proxy rules: $list"

echo "smoke test passed ($ver, nonroot user: $user)"
