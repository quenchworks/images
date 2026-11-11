#!/usr/bin/env bash
# Smoke test for a built frp image. Usage: test.sh <image-ref> [version]
# A real tunnel: frps serves its dashboard on 7500; frpc (same image, frpc entrypoint) dials
# frps on 7000 and exposes that dashboard back through frps on remote port 6000. Reading the
# dashboard through 6000 proves both binaries, the auth token, the control channel and the
# embedded web UI.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
ID="frp-smoke-$$"
cleanup() { docker rm -f "$ID-s" "$ID-c" >/dev/null 2>&1 || true; docker network rm "$ID" >/dev/null 2>&1 || true; rm -rf "${conf:-}"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
for b in frps frpc; do
  v="$(docker run --rm --entrypoint "/usr/bin/$b" "$IMAGE" --version)"
  echo "$b $v"
  [ -z "$WANT" ] || [ "$v" = "$WANT" ] || { echo "$b: expected $WANT"; exit 1; }
done

# configs under $PWD: Docker Desktop cannot mount /tmp
conf="$(mktemp -d "${PWD}/.frp-smoke.XXXXXX")"
cat > "$conf/frps.toml" <<'T'
bindPort = 7000
auth.token = "smoke-token"
webServer.addr = "0.0.0.0"
webServer.port = 7500
webServer.user = "admin"
webServer.password = "smoke-pass"
T
cat > "$conf/frpc.toml" <<T
serverAddr = "$ID-s"
serverPort = 7000
auth.token = "smoke-token"

[[proxies]]
name = "dash"
type = "tcp"
localIP = "$ID-s"
localPort = 7500
remotePort = 6000
T
chmod -R a+rX "$conf"

docker network create "$ID" >/dev/null
docker run -d --name "$ID-s" --network "$ID" --read-only -v "$conf:/conf:ro" \
  -p 127.0.0.1:17500:7500 -p 127.0.0.1:16000:6000 "$IMAGE" -c /conf/frps.toml >/dev/null
docker run -d --name "$ID-c" --network "$ID" --read-only -v "$conf:/conf:ro" \
  --entrypoint /usr/bin/frpc "$IMAGE" -c /conf/frpc.toml >/dev/null

ok=0
for _ in $(seq 1 30); do
  page="$(curl -fsSL -u admin:smoke-pass http://127.0.0.1:16000/ 2>/dev/null || true)"
  grep -qi '<html' <<<"$page" && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "dashboard not reachable through the tunnel"; docker logs "$ID-s" 2>&1 | tail -20; docker logs "$ID-c" 2>&1 | tail -20; exit 1; }
echo "tunnel ok: frps dashboard served through frpc on remote port 6000"

info="$(curl -fsS -u admin:smoke-pass http://127.0.0.1:17500/api/serverinfo)"
grep -q '"version"' <<<"$info" || { echo "serverinfo: $info"; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' -u admin:wrong http://127.0.0.1:17500/api/serverinfo)"
[ "$code" = 401 ] || { echo "wrong password got HTTP $code"; exit 1; }
echo "smoke test passed: tunnel, dashboard API, auth refusal, user 1001"
