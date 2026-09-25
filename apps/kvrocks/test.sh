#!/usr/bin/env bash
# Smoke test for a built kvrocks image. Usage: test.sh <image-ref> [version]
# Real work over the Redis protocol (valkey-cli from the QuenchWorks valkey image): a
# password-protected server answers PING, stores and reads strings, hashes and lists,
# reports its version, and still has the data after a restart on the same volume.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NET="kvrocks-smoke-$$"; NAME="$NET-kv"; VOL="$NET-data"; PW="quench-smoke-pw"
CLI=ghcr.io/quenchworks/images/valkey:9.1.2
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$VOL" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker network create "$NET" >/dev/null
# Docker seeds an empty named volume from the image's /var/lib/kvrocks, which apko owns
# as uid 1001, so the server can write to it without a chown
start() { docker run -d --name "$NAME" --network "$NET" --read-only --tmpfs /tmp -v "$VOL:/var/lib/kvrocks" "$IMAGE" --requirepass "$PW" >/dev/null; }
cli() { docker run --rm --network "$NET" --entrypoint /usr/bin/valkey-cli "$CLI" -h "$NAME" -p 6666 -a "$PW" --no-auth-warning "$@"; }
wait_up() { for _ in $(seq 1 60); do [ "$(cli PING 2>/dev/null)" = PONG ] && return 0; sleep 1; done; docker logs "$NAME" 2>&1 | tail -30; return 1; }

start; wait_up || { echo "kvrocks never answered PING"; exit 1; }

info="$(cli INFO server | tr -d '\r')"
ver="$(echo "$info" | sed -n 's/^kvrocks_version://p')"
echo "kvrocks_version=$ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }
[ "$(cli SET k quench)" = OK ] || { echo "SET failed"; exit 1; }
[ "$(cli GET k)" = quench ] || { echo "GET returned the wrong value"; exit 1; }
cli HSET h f1 v1 f2 v2 >/dev/null; [ "$(cli HGET h f2)" = v2 ] || { echo "hash round trip failed"; exit 1; }
cli RPUSH l a b c >/dev/null; [ "$(cli LRANGE l 0 -1 | tr '\n' ' ')" = "a b c " ] || { echo "list round trip failed"; exit 1; }
[ "$(docker run --rm --network "$NET" --entrypoint /usr/bin/valkey-cli "$CLI" -h "$NAME" -p 6666 PING 2>&1 | head -1)" != PONG ] \
  || { echo "server answered without the password"; exit 1; }

docker rm -f "$NAME" >/dev/null
start; wait_up || { echo "kvrocks did not come back"; exit 1; }
[ "$(cli GET k)" = quench ] || { echo "data lost across a restart"; exit 1; }
echo "smoke test passed (kvrocks ${ver}, uid $user, auth, strings/hashes/lists, data kept across restart)"
