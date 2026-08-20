#!/usr/bin/env bash
# Boot test for a built APISIX Ingress Controller image.
#   usage: test.sh <image-ref> [expected-version]
#
# WHY THIS IS NOT A `--help` TEST
# A Kubernetes controller has no standalone HTTP surface: with no API server it
# calls ctrl.GetConfigOrDie() and exits before any port is bound, so the usual
# "curl the port" pattern cannot work and the catalog's older controller images
# settled for `--help`. `--help` proves almost nothing -- a binary that panics on
# every real code path still prints its flags.
#
# So this test stands up a ~30-line stub API server and boots the controller
# against it. That is enough to get the whole real startup path to run:
#   config file parsed -> kubeconfig loaded -> discovery probed -> scheme
#   registered -> reconcilers built -> manager started -> probes served.
# Then it asserts things only THIS controller can produce:
#   * /healthz returns the body "ok" and /readyz returns 200 on the probe port
#     the injected config file asked for (not the :8081 default), so the config
#     file is really parsed and honored;
#   * /metrics serves controller_runtime_* series, i.e. a controller-runtime
#     manager is actually running, not just a process sitting on a socket;
#   * the stub saw discovery requests for apisix.apache.org/v2 AND
#     gateway.networking.k8s.io/v1 -- the controller's own CRD group and Gateway
#     API. No other binary in the catalog asks for those.
# And it pins the deterministic failure too: with no kubeconfig the image must
# exit non-zero having printed its resolved config and the in-cluster-config
# error, so a silent no-op start can never pass as healthy.
#
# The stub answers only /version and the discovery endpoints, so list/watch fails
# and the caches never sync -- deliberately. Readiness of the CONTROLLER LOGIC is
# the chart's kind-install gate's job; this proves the IMAGE boots and serves.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECTED="${2:-}"

# The stub runs on the HOST and the container reaches it through
# --add-host=...:host-gateway (works on plain dockerd and on Docker Desktop, where
# --network host would only see the VM's loopback). Config files go in with
# `docker cp`, not a bind mount, because a bind mount of /tmp is refused under
# Docker Desktop's file sharing. Ports are picked free at runtime -- a fixed port
# collides with whatever else the build host happens to be running.
command -v python3 >/dev/null || { echo "python3 is required for the stub API server"; exit 1; }

freeport() { python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()'; }
API_PORT="$(freeport)"
PROBE_PORT="$(freeport)"
METRICS_PORT="$(freeport)"

NAME="quench-aic-smoke-$$"
WORK="$(mktemp -d)"
STUB_PID=""
cleanup() {
  docker rm -f "$NAME" "${NAME}-nokube" >/dev/null 2>&1 || true
  [ -n "$STUB_PID" ] && kill "$STUB_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

# ---------------------------------------------------------------- 1. version
echo "checking the stamped version"
vout="$(docker run --rm "$IMAGE" version --long 2>&1)" || { echo "version --long failed"; echo "$vout"; exit 1; }
echo "$vout"
ver="$(awk -F': ' '/^Version:/{print $2; exit}' <<<"$vout")"
sha="$(awk -F': ' '/^Git SHA:/{print $2; exit}' <<<"$vout")"
case "$ver" in ""|0.0.0|*dev*|unknown) echo "version not stamped: '$ver'"; exit 1 ;; esac
[[ "$sha" =~ ^[0-9a-f]{40}$ ]] || { echo "git revision not stamped: '$sha'"; exit 1; }
if [ -n "$EXPECTED" ] && [ "$ver" != "$EXPECTED" ]; then
  echo "version mismatch: image says '$ver', expected '$EXPECTED'"; exit 1
fi

# --------------------------------------------- 2. deterministic hard failure
# No kubeconfig, no service account: must exit non-zero AND say why.
echo "checking it fails legibly with no kubeconfig"
# Detached + `docker inspect`, not `timeout docker run`: an image that hangs keeps
# the attached CLI alive and SIGTERM does not necessarily reach PID 1, so the
# straightforward version can hang instead of failing.
docker run -d --name "${NAME}-nokube" "$IMAGE" >/dev/null
nstate=""
for i in $(seq 1 30); do
  nstate="$(docker inspect -f '{{.State.Status}}' "${NAME}-nokube")"
  [ "$nstate" = exited ] && break
  sleep 1
done
nout="$(docker logs "${NAME}-nokube" 2>&1)"
[ "$nstate" = exited ] \
  || { echo "did not exit within 30s with no kubeconfig; it must fail fast, not hang"; echo "$nout"; exit 1; }
nrc="$(docker inspect -f '{{.State.ExitCode}}' "${NAME}-nokube")"
[ "$nrc" -ne 0 ] || { echo "expected a non-zero exit with no kubeconfig, got 0"; echo "$nout"; exit 1; }
grep -q "controller start configuration" <<<"$nout" \
  || { echo "did not print its resolved configuration before failing"; echo "$nout"; exit 1; }
grep -q "unable to load in-cluster configuration" <<<"$nout" \
  || { echo "unexpected failure mode with no kubeconfig"; echo "$nout"; exit 1; }

# ------------------------------------------------- 3. real boot on a stub API
cat > "$WORK/stub.py" <<'PY'
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

LOG = open(sys.argv[2], "a", buffering=1)

BODIES = {
    "/version": {"major": "1", "minor": "31", "gitVersion": "v1.31.0", "platform": "linux/amd64"},
    "/api": {"kind": "APIVersions", "versions": ["v1"], "serverAddressByClientCIDRs": []},
    "/apis": {"kind": "APIGroupList", "apiVersion": "v1", "groups": []},
}

class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_GET(self):
        path = self.path.split("?")[0]
        LOG.write(path + "\n")
        body = BODIES.get(path)
        code = 200 if body else 404
        if not body:
            body = {"kind": "Status", "apiVersion": "v1", "status": "Failure",
                    "reason": "NotFound", "message": "stub", "code": 404}
        raw = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

HTTPServer(("0.0.0.0", int(sys.argv[1])), H).serve_forever()
PY

mkdir -p "$WORK/conf"
cat > "$WORK/conf/kubeconfig" <<EOF
apiVersion: v1
kind: Config
clusters:
  - name: stub
    cluster:
      server: http://stub-api:${API_PORT}
contexts:
  - name: stub
    context:
      cluster: stub
      user: stub
current-context: stub
users:
  - name: stub
    user: {}
EOF

# Non-default ports on purpose: if the controller ignored this file the probes
# would come up on :8081 and the assertions below would fail.
cat > "$WORK/conf/config.yaml" <<EOF
log_level: info
probe_addr: ":${PROBE_PORT}"
metrics_addr: ":${METRICS_PORT}"
leader_election:
  disable: true
provider:
  type: apisix
  sync_period: 1h
EOF

chmod 755 "$WORK/conf"; chmod 644 "$WORK/conf"/*   # the image runs as uid 1001

python3 "$WORK/stub.py" "$API_PORT" "$WORK/api.log" &
STUB_PID=$!
for i in $(seq 1 10); do
  curl -fsS -m 2 "http://127.0.0.1:${API_PORT}/version" >/dev/null 2>&1 && break
  [ "$i" = 10 ] && { echo "stub API server did not start"; exit 1; }
  sleep 1
done

echo "booting the controller against the stub API server"
docker create --name "$NAME" \
  --add-host "stub-api:host-gateway" \
  -p "127.0.0.1:${PROBE_PORT}:${PROBE_PORT}" \
  -p "127.0.0.1:${METRICS_PORT}:${METRICS_PORT}" \
  -e KUBECONFIG=/aic-conf/kubeconfig \
  "$IMAGE" -c /aic-conf/config.yaml >/dev/null
# `docker cp` of a directory creates the destination, which a shell-free image
# cannot do for itself and a bind mount cannot do under Docker Desktop.
docker cp "$WORK/conf" "$NAME:/aic-conf"
docker start "$NAME" >/dev/null

for i in $(seq 1 40); do
  [ "$(curl -s -m 2 "http://127.0.0.1:${PROBE_PORT}/healthz" || true)" = "ok" ] && break
  if [ "$i" = 40 ] || ! docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null | grep -q true; then
    echo "controller never served /healthz on :${PROBE_PORT}"; docker logs "$NAME" | tail -30; exit 1
  fi
  sleep 1
done
echo "/healthz -> ok"

code="$(curl -s -m 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PROBE_PORT}/readyz")"
[ "$code" = "200" ] || { echo "/readyz returned $code"; docker logs "$NAME" | tail -30; exit 1; }

# Capture first: `curl | grep -q` closes the pipe early and pipefail then reports
# the SIGPIPE'd curl as a failure even when the pattern matched.
mout="$(curl -fsS -m 5 "http://127.0.0.1:${METRICS_PORT}/metrics" 2>/dev/null || true)"
grep -q '^controller_runtime_' <<<"$mout" \
  || { echo "no controller_runtime_* metrics on :${METRICS_PORT}"; docker logs "$NAME" | tail -30; exit 1; }
echo "/readyz -> 200, /metrics serves controller_runtime_* series"

# The controller must have gone looking for its OWN CRDs and for Gateway API.
for want in /apis/apisix.apache.org/v2 /apis/gateway.networking.k8s.io/v1; do
  grep -qx "$want" "$WORK/api.log" \
    || { echo "controller never probed $want"; echo "--- stub saw:"; sort -u "$WORK/api.log"; docker logs "$NAME" | tail -30; exit 1; }
done
echo "discovery probed apisix.apache.org/v2 and gateway.networking.k8s.io/v1"

# must run as the nonroot user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "boot test passed (version $ver, revision ${sha:0:12}, nonroot user: $user)"
