#!/usr/bin/env bash
# Boot test for the Teleport image. Usage: test.sh <image-ref> [version]
# Starts a single-node auth + proxy from `teleport configure`, then checks what a port probe
# would miss: the proxy's /webapi/ping reports the version, the web UI was built and embedded
# (the bundle is served, not a placeholder), and tctl reaches the auth service.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
T="tp-$$-$RANDOM"
trap 'docker rm -f "$T" >/dev/null 2>&1 || true' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" version)"
echo "$ver"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$ver" || { echo "expected v$WANT"; exit 1; }

# teleport start has no --cluster-name or --config-string: mount a minimal v3 config. The data dir
# is a uid-1001 tmpfs. (TMPDIR is a mountable dir under the gate; plain /tmp is not on Docker Desktop.)
WORK="$(mktemp -d)"
trap 'docker rm -f "$T" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
cat > "$WORK/teleport.yaml" <<'YAML'
version: v3
teleport:
  nodename: quench
  data_dir: /var/lib/teleport
  log:
    output: stderr
    severity: INFO
auth_service:
  enabled: true
  cluster_name: quench.test
proxy_service:
  enabled: true
  web_listen_addr: 0.0.0.0:3080
  public_addr: localhost:3080
ssh_service:
  enabled: false
YAML
chmod 644 "$WORK/teleport.yaml"
docker run -d --name "$T" -p 127.0.0.1:13080:3080 --tmpfs /var/lib/teleport:uid=1001,gid=1001 \
  -v "$WORK/teleport.yaml:/etc/teleport/teleport.yaml:ro" \
  "$IMAGE" start --config=/etc/teleport/teleport.yaml >/dev/null
ok=0
for _ in $(seq 1 60); do
  out="$(curl -ksS https://127.0.0.1:13080/webapi/ping 2>/dev/null || true)"
  grep -q '"server_version"' <<<"$out" && { ok=1; break; }
  [ "$(docker inspect -f '{{.State.Running}}' "$T")" = true ] || break
  sleep 3
done
[ "$ok" = 1 ] || { echo "proxy never answered /webapi/ping"; docker logs "$T" 2>&1 | tail -30; exit 1; }
echo "ping: $(head -c 300 <<<"$out")"
[ -z "$WANT" ] || grep -q "\"server_version\":\"$WANT\"" <<<"$out" || { echo "ping reports another version"; exit 1; }

page="$(curl -ksS https://127.0.0.1:13080/web/login)"
grep -qE '<script[^>]+src="/web/app/' <<<"$page" || { echo "web UI bundle not served: $(head -c 300 <<<"$page")"; exit 1; }
asset="$(grep -oE '/web/app/[^"]+\.js' <<<"$page" | head -1)"
# the bundle is embedded Brotli-only (app.js.br): ask the way a browser does
code="$(curl -ksS -H 'Accept-Encoding: br' -o /dev/null -w '%{http_code}' "https://127.0.0.1:13080$asset")"
[ "$code" = 200 ] || { echo "UI asset $asset answered $code"; exit 1; }

st="$(docker exec "$T" tctl --config=/etc/teleport/teleport.yaml status 2>&1)"
grep -q 'quench.test' <<<"$st" || { echo "tctl status failed: $st"; exit 1; }
echo "smoke test passed (teleport ${WANT:-?}, uid $user, web UI served, tctl reached auth)"
