#!/usr/bin/env bash
# Smoke test for a built Concourse image. Usage: test.sh <image-ref> [version]
# Runs the web node on a read-only rootfs against a throwaway PostgreSQL, with keys the
# image generates itself and a local admin user. Requires the API to report the version,
# the embedded UI to serve, and a password login to return a token.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
T="concourse-smoke-$$"
NET="$T-net"
PORT=18080
KEYS="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$T" "$T-pg" "$T-worker" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$KEYS"; }
trap cleanup EXIT
fail() { echo "$1"; docker logs "$T" 2>&1 | tail -30; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version)"
[ -z "$WANT" ] || [ "$v" = "$WANT" ] || { echo "version mismatch: $v"; exit 1; }

chmod 0777 "$KEYS"
docker run --rm -v "$KEYS:/keys" "$IMAGE" generate-key -t rsa -f /keys/session_signing_key >/dev/null
docker run --rm -v "$KEYS:/keys" "$IMAGE" generate-key -t ssh -f /keys/tsa_host_key >/dev/null
docker run --rm -v "$KEYS:/keys" "$IMAGE" generate-key -t ssh -f /keys/worker_key >/dev/null
chmod -R a+r "$KEYS"

docker network create "$NET" >/dev/null
docker run -d --name "$T-pg" --network "$NET" -e POSTGRES_USER=concourse -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=concourse postgres:17 >/dev/null
for _ in $(seq 1 60); do docker exec "$T-pg" psql -U concourse -d concourse -c 'select 1' >/dev/null 2>&1 && break; sleep 1; done

docker run -d --name "$T" --network "$NET" --read-only --tmpfs /tmp -v "$KEYS:/keys:ro" -p "127.0.0.1:$PORT:8080" \
  -e CONCOURSE_POSTGRES_HOST="$T-pg" -e CONCOURSE_POSTGRES_USER=concourse -e CONCOURSE_POSTGRES_PASSWORD=pw \
  -e CONCOURSE_POSTGRES_DATABASE=concourse -e CONCOURSE_EXTERNAL_URL="http://127.0.0.1:$PORT" \
  -e CONCOURSE_ADD_LOCAL_USER=smoke:Smoke-Pass-2026 -e CONCOURSE_MAIN_TEAM_LOCAL_USER=smoke \
  -e CONCOURSE_SESSION_SIGNING_KEY=/keys/session_signing_key -e CONCOURSE_TSA_HOST_KEY=/keys/tsa_host_key \
  -e CONCOURSE_TSA_AUTHORIZED_KEYS=/keys/worker_key.pub \
  "$IMAGE" web >/dev/null
B="http://127.0.0.1:$PORT"
info=""
for _ in $(seq 1 90); do info="$(curl -fsS "$B/api/v1/info" 2>/dev/null)" && break; sleep 1; done
[ -n "$info" ] || fail "web never answered /api/v1/info"
echo "info: $info"
[ -z "$WANT" ] || grep -q "\"version\":\"$WANT\"" <<<"$info" || fail "API does not report $WANT"
page="$(curl -fsS "$B/")"
grep -q 'elm.min.js' <<<"$page" || fail "the embedded UI did not serve"
curl -fsS -o /dev/null "$B/public/elm.min.js" || fail "the compiled UI bundle did not serve"
tok="$(curl -fsS -u fly:Zmx5 -d 'grant_type=password&username=smoke&password=Smoke-Pass-2026&scope=openid+profile+email+federated:id+groups' "$B/sky/issuer/token")"
grep -q '"access_token"' <<<"$tok" || fail "password login did not return a token: $tok"
teams="$(curl -fsS -H "Authorization: Bearer $(sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p' <<<"$tok")" "$B/api/v1/teams")"
grep -q '"name":"main"' <<<"$teams" || fail "the main team is not visible to its local user"

# worker: root and privileged (it creates containers), registered over the TSA on :2222
docker run -d --name "$T-worker" --network "$NET" --privileged --user 0 --tmpfs /tmp -v "$KEYS:/keys:ro" \
  -e CONCOURSE_TSA_HOST="$T:2222" -e CONCOURSE_TSA_PUBLIC_KEY=/keys/tsa_host_key.pub \
  -e CONCOURSE_TSA_WORKER_PRIVATE_KEY=/keys/worker_key -e CONCOURSE_RUNTIME=containerd \
  -e CONCOURSE_WORK_DIR=/tmp/worker -e CONCOURSE_CONTAINERD_CNI_PLUGINS_DIR=/usr/bin \
  -e CONCOURSE_CONTAINERD_DNS_SERVER=8.8.8.8 \
  "$IMAGE" worker >/dev/null
AUTH="Authorization: Bearer $(sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p' <<<"$tok")"
state=""
for _ in $(seq 1 90); do
  w="$(curl -fsS -H "$AUTH" "$B/api/v1/workers" 2>/dev/null || true)"
  state="$(sed -n 's/.*"state":"\([a-z]*\)".*/\1/p' <<<"$w" | head -1)"
  [ "$state" = running ] && break
  sleep 2
done
[ "$state" = running ] || { docker logs "$T-worker" 2>&1 | tail -30; fail "the worker never reached running (state: ${state:-none})"; }
echo "worker registered and running: $(sed -n 's/.*"platform":"\([a-z]*\)".*/\1/p' <<<"$w" | head -1)"
echo "smoke test passed (concourse ${WANT:-?}, uid $user, API, embedded UI, local login, main team, worker running)"
