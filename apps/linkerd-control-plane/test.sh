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

# WHAT THIS TEST CAN AND CANNOT PROVE, as of edge-26.8.4.
#
# It used to drive the admin server: /ping, /metrics, /ready. That is no longer
# reachable without a real cluster. The destination controller makes TWO fatal
# API calls during startup, and disabling the first only exposes the second:
#   Failed to start with EndpointSlices enabled: ... /apis/discovery.k8s.io/v1
#   Failed to initialize K8s API: ... /apis/authorization.k8s.io/v1/selfsubjectaccessreviews
# The admin server is started before both and dies with the process, so nothing
# is servable against a fake kubeconfig no matter which flags are passed.
#
# So this is a BOOT test now, not a serving test: it proves the binary runs, is
# linked correctly, reports the right version, gets as far as starting its admin
# server, and then fails for the ONE documented reason rather than a link error,
# a missing library, or a panic. Functional proof lives in the chart's kind
# install gate, which has a real API server.
sleep 5
logs="$(docker logs "$NAME" 2>&1)"

echo "checking the binary reached its admin server"
echo "$logs" | grep -q 'starting admin server on :9996' \
  || { echo "never reached the admin server:"; echo "$logs"; exit 1; }

echo "checking it died for the expected reason, not something else"
echo "$logs" | grep -q 'Failed to initialize K8s API' \
  || { echo "did not fail the documented way; read the log before trusting this image:"; echo "$logs"; exit 1; }

echo "checking nothing panicked"
if echo "$logs" | grep -qE 'panic:|SIGSEGV|no such file or directory|cannot execute'; then
  echo "the binary did not start cleanly:"; echo "$logs"; exit 1
fi

# The version is not a CLI flag on this binary -- it's LINKERD_CONTAINER_VERSION_OVERRIDE,
# baked into the image. Read it from the IMAGE, not a running container: the
# container has exited by now.
if [ -n "$EXPECT_VER" ]; then
  echo "$logs" | grep -q "running version ${EXPECT_VER}" \
    || { echo "expected 'running version ${EXPECT_VER}' in the log:"; echo "$logs"; exit 1; }
fi

# must run as the nonroot linkerd-control-plane user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "boot test passed (binary runs, version ${EXPECT_VER:-unchecked}, reached the admin server, failed only on the unreachable API server, nonroot user: $user)"
