#!/usr/bin/env bash
# Boot test for a built ADC image.
#   usage: test.sh <image-ref> [expected-version]
#
# WHY THIS IS NOT A `--version` TEST
# adc's whole reason for being in this catalog is `server` mode: it is the
# `adc-server` sidecar of apisix-ingress-controller, and it fails in ways a
# version string cannot see. So this test starts the real server the way
# upstream's Deployment does (config/manager/manager.yaml at controller tag
# 2.2.0) and asserts everything the chart depends on:
#
#   1. the stamped version (adc hardcodes it in source, so this is a real pin);
#   2. the ADC_RUNNING_MODE=ingress gate, IN BOTH DIRECTIONS -- `server` exists
#      only in ingress mode, and a chart that forgets the env var gets
#      `unknown value: unknown command 'server'` at runtime. Pinning the negative
#      case is what stops that regressing quietly;
#   3. the status port answers GET /healthz/ready with 200 "OK" -- upstream's
#      liveness AND readiness probe for this container;
#   4. the Unix socket is really created, and with the permissions the chart's
#      fsGroup has to work with: a socket, mode 0660, owned by uid 1001.
#      0660 is set by adc itself (fs.chmodSync on bind), which is exactly why
#      the pod needs fsGroup 2000 to let the controller in;
#   5. the socket serves the ADC DATA API, not just an open file descriptor:
#      PUT /sync with a bogus body comes back 400 with adc's own zod complaint
#      about the missing `task` field. Nothing else in the catalog answers that;
#   6. the two listeners are NOT interchangeable -- /healthz/ready is absent from
#      the socket and /sync is absent from the status port. Probe the wrong one
#      and you get a 404, so this pins which is which for the chart.
#
# Everything after the boot runs through `docker exec <ctr> /usr/bin/node -e`:
# the image has no shell, and node is the one interpreter it does have. That also
# avoids a bind mount of the socket dir, which Docker Desktop refuses for /tmp.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECTED="${2:-}"

SOCK=/sockets/adc.sock
NAME="quench-adc-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

command -v python3 >/dev/null || { echo "python3 is required to pick a free port"; exit 1; }
STATUS_PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')"

# ---------------------------------------------------------------- 1. version
echo "checking the stamped version"
ver="$(docker run --rm "$IMAGE" -v 2>&1 | tr -d '\r' | tail -1)"
echo "adc -v -> $ver"
case "$ver" in ""|0.0.0|*dev*|*unknown*) echo "version not stamped: '$ver'"; exit 1 ;; esac
if [ -n "$EXPECTED" ] && [ "$ver" != "$EXPECTED" ]; then
  echo "version mismatch: image says '$ver', expected '$EXPECTED'"; exit 1
fi

# ------------------------------------------- 2. the ADC_RUNNING_MODE gate
# Without ADC_RUNNING_MODE=ingress there is no `server` subcommand at all. This
# is the single most likely chart mistake, so pin it as a hard expectation.
echo "checking the ADC_RUNNING_MODE=ingress gate"
if out="$(docker run --rm "$IMAGE" server --help 2>&1)"; then
  echo "FATAL: 'server' worked WITHOUT ADC_RUNNING_MODE=ingress; the gate moved upstream"
  echo "$out"; exit 1
fi
grep -q "unknown command 'server'" <<<"$out" \
  || { echo "unexpected failure mode without ADC_RUNNING_MODE"; echo "$out"; exit 1; }
gout="$(docker run --rm -e ADC_RUNNING_MODE=ingress "$IMAGE" server --help 2>&1)"
grep -q -- '--listen-status' <<<"$gout" \
  || { echo "'server --help' in ingress mode did not describe --listen-status"; echo "$gout"; exit 1; }
echo "gate holds: server exists only with ADC_RUNNING_MODE=ingress"

# ------------------------------------------------------ 3. boot the sidecar
# Same env + args as upstream's adc-server container.
echo "booting adc server on unix:${SOCK} (status :${STATUS_PORT})"
docker run -d --name "$NAME" \
  -e ADC_RUNNING_MODE=ingress \
  -e ADC_EXPERIMENTAL_FEATURE_FLAGS=remote-state-file,parallel-backend-request \
  -e ADC_INGRESS_LOG_LEVEL=info \
  -p "127.0.0.1:${STATUS_PORT}:3001" \
  "$IMAGE" server --listen "unix:${SOCK}" --listen-status 3001 >/dev/null

for i in $(seq 1 40); do
  [ "$(curl -s -m 2 "http://127.0.0.1:${STATUS_PORT}/healthz/ready" || true)" = "OK" ] && break
  if [ "$i" = 40 ] || ! docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null | grep -q true; then
    echo "adc never served /healthz/ready on :3001"; docker logs "$NAME" 2>&1 | tail -30; exit 1
  fi
  sleep 1
done
code="$(curl -s -m 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${STATUS_PORT}/healthz/ready")"
[ "$code" = "200" ] || { echo "/healthz/ready returned $code"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
echo "status :3001 /healthz/ready -> 200 OK"

# It must have said which socket it bound, not just started something.
docker logs "$NAME" 2>&1 | grep -q "ADC server is running on: ${SOCK}" \
  || { echo "no 'ADC server is running on: ${SOCK}' line in the log"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }

# The status port must NOT carry the data API (the chart probes 3001, and a
# listener that answered everything would hide a misrouted socket).
scode="$(curl -s -m 5 -o /dev/null -w '%{http_code}' -X PUT \
  -H 'content-type: application/json' -d '{}' "http://127.0.0.1:${STATUS_PORT}/sync")"
[ "$scode" = "404" ] || { echo "status port answered PUT /sync with $scode, expected 404"; exit 1; }

# --------------------------------- 4+5+6. the socket: perms and live HTTP
# No shell in the image; node is the interpreter, and it is the same client
# transport the controller uses (HTTP over an AF_UNIX socket).
read -r -d '' PROBE <<'EOJS' || true
const fs = require('node:fs'), http = require('node:http');
const SOCK = '/sockets/adc.sock';
const st = fs.statSync(SOCK);
const mode = (st.mode & 0o777).toString(8);
if (!st.isSocket()) { console.error('FAIL: ' + SOCK + ' is not a socket'); process.exit(1); }
if (mode !== '660') { console.error('FAIL: socket mode is 0' + mode + ', expected 0660'); process.exit(1); }
if (st.uid !== 1001) { console.error('FAIL: socket uid is ' + st.uid + ', expected 1001'); process.exit(1); }
console.log('SOCKET ok: mode=0' + mode + ' uid=' + st.uid + ' gid=' + st.gid);

const req = (path, method, body) => new Promise((res) => {
  const q = http.request({ socketPath: SOCK, path, method,
    headers: { 'content-type': 'application/json' } },
    (r) => { let d = ''; r.on('data', (c) => d += c); r.on('end', () => res([r.statusCode, d])); });
  q.on('error', (e) => res(['ERR', e.message]));
  if (body) q.write(body);
  q.end();
});

(async () => {
  const [sc, sb] = await req('/sync', 'PUT', '{}');
  if (sc !== 400) { console.error('FAIL: PUT /sync gave ' + sc + ' ' + sb); process.exit(1); }
  if (!sb.includes('task')) { console.error('FAIL: PUT /sync 400 body is not ADC validation: ' + sb); process.exit(1); }
  const [vc] = await req('/validate', 'PUT', '{}');
  if (vc !== 400) { console.error('FAIL: PUT /validate gave ' + vc); process.exit(1); }
  const [hc] = await req('/healthz/ready', 'GET');
  if (hc !== 404) { console.error('FAIL: data socket served /healthz/ready with ' + hc + '; that belongs to the status port only'); process.exit(1); }
  console.log('SOCKET api ok: PUT /sync 400 (zod: task), PUT /validate 400, GET /healthz/ready 404');
})();
EOJS

docker exec "$NAME" /usr/bin/node -e "$PROBE" \
  || { echo "socket probe failed"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }

# --------------------------------------------------------------- 7. nonroot
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "boot test passed (version $ver, nonroot user: $user)"
