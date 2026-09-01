#!/usr/bin/env bash
# Smoke test for a built Envoy Gateway image.
# Usage: test.sh <image-ref> [expected-version]
#
# Envoy Gateway is a Kubernetes controller: `server` does nothing useful without an API
# server, so the usual "curl the app's port" pattern does not apply. This test asserts
# three things that DO hold in a bare `docker run`, none of which a broken binary can
# fake:
#
#   1. The shutdown-manager HTTP server really boots and serves /healthz on :19002.
#      This is not a contrivance -- the controller injects THIS image as a
#      `shutdown-manager` sidecar into every managed Envoy pod, generated as
#      Command ["envoy-gateway"] Args ["envoy","shutdown-manager"], and kubelet
#      liveness/readiness probes that exact path (internal/cmd/envoy/shutdown_manager.go,
#      internal/infrastructure/kubernetes/proxy/resource.go). It needs no cluster.
#   2. `server` reaches the point of starting its runners and then FAILS, non-zero,
#      because there is no API server to talk to. That walks config load, logger setup
#      and runner wiring -- a binary that cannot initialise exits before "Start runners".
#      A hang or a zero exit is a failure here.
#   3. `version` reports the release stamped by the -X ldflags, plus the Envoy and
#      Gateway API versions this build is coupled to.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
NAME="quench-envoy-gateway-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "1/4 shutdown-manager boots and answers /healthz on :19002"
docker run -d --name "$NAME" -p 127.0.0.1:19002:19002 "$IMAGE" envoy shutdown-manager >/dev/null
for i in $(seq 1 30); do
  curl -fsS -o /dev/null http://127.0.0.1:19002/healthz && break
  [ "$i" = 30 ] && { echo "shutdown-manager never answered /healthz"; docker logs "$NAME"; exit 1; }
  sleep 1
done
docker logs "$NAME" 2>&1 | grep -q "starting shutdown manager" \
  || { echo "no 'starting shutdown manager' in the logs"; docker logs "$NAME"; exit 1; }
docker rm -f "$NAME" >/dev/null

echo "2/4 server starts its runners, then fails non-zero without an API server"
out="$(timeout 120 docker run --rm "$IMAGE" server 2>&1)" && rc=0 || rc=$?
[ "$rc" -ne 0 ] || { echo "server exited 0 with no cluster; expected a failure"; echo "$out"; exit 1; }
[ "$rc" -ne 124 ] || { echo "server hung instead of failing fast"; echo "$out"; exit 1; }
echo "$out" | grep -q "Start runners" \
  || { echo "server never reached 'Start runners' (rc=$rc)"; echo "$out"; exit 1; }
echo "  server exited $rc after starting runners, as expected"

echo "3/4 version is stamped by the ldflags"
ver_out="$(docker run --rm "$IMAGE" version)"
echo "$ver_out" | sed 's/^/  /'
eg_ver="$(echo "$ver_out" | awk '/^ENVOY_GATEWAY_VERSION:/{print $2}')"
proxy_ver="$(echo "$ver_out" | awk '/^ENVOY_PROXY_VERSION:/{print $2}')"
gwapi_ver="$(echo "$ver_out" | awk '/^GATEWAYAPI_VERSION:/{print $2}')"
case "$eg_ver" in
  ""|v0.0.0|*dev*) echo "envoy-gateway version not stamped: '$eg_ver'"; exit 1 ;;
esac
if [ -n "$WANT" ] && [ "$eg_ver" != "v${WANT}" ]; then
  echo "expected version v${WANT}, got '$eg_ver'"; exit 1
fi
# The data plane and CRD versions this control plane is coupled to. Empty means the
# build lost its module info (a stripped/rebuilt binary), which would also mean the
# chart cannot tell which envoy image and Gateway API CRD bundle to pin.
# Two shapes are both legitimate. Through 1.8.x upstream's default proxy ref was a
# plain tag, so this stamped "distroless-v1.38.3". From 1.9.0 the default is pinned
# by digest (envoyproxy/envoy:distroless-v1.39.1@sha256:<hex>) and upstream's own
# version code takes the ref's tag by splitting on ':', which cuts the digest in
# half -- so 1.9.1 legitimately reports "distroless-v1.39.1@sha256" (verified by
# running the built 1.9.1 image; the trailing hex really is absent from the binary's
# output, it is not truncated by this test). Accept both and keep asserting the
# semver, which is the part the chart needs to pin the data plane.
echo "$proxy_ver" | grep -qE '^distroless-v[0-9]+\.[0-9]+\.[0-9]+(@sha256(:[0-9a-f]{64})?)?$' \
  || { echo "ENVOY_PROXY_VERSION looks wrong: '$proxy_ver'"; exit 1; }
echo "$gwapi_ver" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$' \
  || { echo "GATEWAYAPI_VERSION missing (lost build info): '$gwapi_ver'"; exit 1; }

echo "4/4 runs as the nonroot envoy-gateway user"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (envoy-gateway $eg_ver, data plane $proxy_ver, gateway-api $gwapi_ver, nonroot $user)"
