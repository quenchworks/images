#!/usr/bin/env bash
# Smoke test for a built linkerd-control-plane image. Usage: test.sh <image-ref> [expected-version]
#
# `controller` is a Kubernetes control plane binary: a full "does it manage a
# mesh" test needs a real cluster (that's the chart's kind install gate, not
# built in this task). But this must still prove something REAL, not just
# --version (there is no --version flag on this binary at all -- version is
# reported only via the running process, see below).
#
# The image's default command runs `controller destination --kubeconfig=...`
# against a baked-in, syntactically-valid but UNREACHABLE kubeconfig (see
# melange.yaml). Reading controller/cmd/destination/main.go: the admin HTTP
# server (metrics_addr, default :9996) is started in its own goroutine BEFORE
# the k8s client is even constructed, and the k8s client construction itself
# only PARSES the kubeconfig (no dial). The thing that blocks main() forever
# without a real cluster is the later `k8sAPI.Sync(nil)` call. So a real,
# unstubbed destination process comes up, its admin server binds for real, and
# stays reachable while informers retry in the background -- same shape as
# istiod's proof, verified against this binary's own source rather than
# assumed from istiod's.
#
# Endpoints hit (from pkg/admin/admin.go, read directly, not guessed):
#   :9996/ping     -- literal "pong\n", proves the handler is live at all
#   :9996/metrics  -- promhttp.Handler(): a real Prometheus exposition body
#   :9996/ready    -- flips only once k8s caches sync; with no reachable
#                     cluster it legitimately never does, so this test checks
#                     REACHABILITY (any HTTP status), not a 200, matching
#                     istiod's test.sh reasoning for the same shape of endpoint.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"
NAME="quench-linkerd-control-plane-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
# EndpointSlices is ON by default and, since edge-26.x, its API-access probe is
# fatal at startup: with an unreachable kubeconfig the destination controller
# dies on
#   Failed to start with EndpointSlices enabled: ... dial tcp 127.0.0.1:6443:
#   connect: connection refused
# before the admin server can serve, so the boot test cannot use the defaults.
# Turning it off also forces -enable-ipv6=false, because main.go Fatals on
# "If --enable-ipv6=true then --enable-endpoint-slices needs to be true"
# (controller/cmd/destination/main.go:74). Both flags are for THIS boot test
# only; the chart runs the controller with upstream's defaults.
docker run -d --name "$NAME" -p 127.0.0.1:9996:9996 "$IMAGE" \
  -enable-endpoint-slices=false -enable-ipv6=false >/dev/null

echo "waiting for the admin server (:9996) to come up"
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:9996/ping >/dev/null 2>&1; then
    break
  fi
  if [ "$i" = 30 ]; then
    echo "admin server did not come up"; docker logs "$NAME"; exit 1
  fi
  sleep 1
done

echo "checking /ping"
pong="$(curl -fsS http://127.0.0.1:9996/ping)"
[ "$pong" = "pong" ] || { echo "unexpected /ping body: '$pong'"; exit 1; }

echo "checking /metrics is a live Prometheus exporter"
metrics="$(curl -fsS http://127.0.0.1:9996/metrics)"
echo "$metrics" | grep -q '^# HELP ' \
  || { echo "no Prometheus exposition format in /metrics"; echo "$metrics" | head -20; exit 1; }

echo "checking /ready is reachable (a non-200 is expected: no real cluster to sync against)"
if ! curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:9996/ready | grep -qE '^[0-9]{3}$'; then
  echo "/ready is not reachable at all"; docker logs "$NAME"; exit 1
fi

# The version is not a CLI flag on this binary -- it's LINKERD_CONTAINER_VERSION_OVERRIDE,
# baked into the image and readable back out of the running container's env.
if [ -n "$EXPECT_VER" ]; then
  ver="$(docker exec "$NAME" printenv LINKERD_CONTAINER_VERSION_OVERRIDE || true)"
  [ "$ver" = "$EXPECT_VER" ] || { echo "expected version '$EXPECT_VER', got '$ver'"; exit 1; }
fi

# must run as the nonroot linkerd-control-plane user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (admin server live: /ping=pong, /metrics has real Prometheus output, /ready reachable, nonroot user: $user)"
