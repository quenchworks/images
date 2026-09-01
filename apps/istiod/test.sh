#!/usr/bin/env bash
# Smoke test for a built istiod image. Usage: test.sh <image-ref> [expected-version]
#
# istiod is a Kubernetes control plane: a full "does it manage a mesh" test needs
# a real cluster (that's the chart's kind install gate, not built in this task).
# But this must still prove something REAL, not just --version.
#
# The image's default command runs `pilot-discovery discovery
# --kubeconfig=/etc/istio/kubeconfig` against a baked-in, syntactically-valid but
# UNREACHABLE kubeconfig (see melange.yaml) -- a real, unstubbed pilot-discovery
# binary talking to a cluster it can't reach, same as any istiod pod before its
# RBAC/networking is wired up. (--registries=Mock was tried first and actually
# SIGSEGV'd: initServiceControllers unconditionally builds a serviceentry.Controller
# against a multicluster.Controller that is only non-nil once a kube client exists,
# verified by running it.)
#
# WHAT IS AND IS NOT REACHABLE IN THAT STATE -- measured, not assumed:
# pilot-discovery starts its admin/discovery HTTP server (:8080) EARLY, then blocks
# in the config controller waiting for the initial ConfigMap cache sync, which never
# completes against an unreachable API server. The monitoring server (:15014,
# /version + /metrics + pilot_info) and ControlZ (:9876, /versionz) are both started
# only AFTER that sync returns, so cluster-less they never bind -- an earlier version
# of this test asserted on :15014 and could therefore never pass. (Confirmed by
# running the built image: :8080/debug answers 200 while the log repeats
# "waiting for sync... name=ConfigMap_istio" indefinitely; :15014 and :9876 give
# curl(52)/no listener.) So assert the surface that IS live:
#   :8080/debug  -- the real Pilot Debug Console index, rendered from the xDS debug
#                   handler registry (adsz/syncz/configz/...). A stub that only
#                   parsed flags could not serve it.
#   :8080/ready  -- 503 with the unready reason: the readiness machinery is live and
#                   correctly reporting "no cluster" rather than refusing connections.
#   the log      -- "waiting for sync" proves the real kube config controller started
#                   and is genuinely attempting the API server.
# The ldflag-stamped version is read from the binary itself, since cluster-less there
# is no live handler that serves it.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"
NAME="quench-istiod-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" -p 127.0.0.1:8080:8080 "$IMAGE" >/dev/null

echo "waiting for the discovery/admin HTTP server (:8080) to come up"
for i in $(seq 1 60); do
  if [ "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 3 http://127.0.0.1:8080/debug 2>/dev/null)" = "200" ]; then
    break
  fi
  if [ "$i" = 60 ]; then
    echo "discovery HTTP server (:8080) did not come up"; docker logs "$NAME"; exit 1
  fi
  sleep 1
done

echo "checking /debug is the real Pilot Debug Console (xDS handler registry)"
dbg="$(curl -fsS http://127.0.0.1:8080/debug)"
for want in 'Pilot Debug Console' '/debug/adsz' '/debug/syncz' '/debug/configz'; do
  case "$dbg" in
    *"$want"*) ;;
    *) echo "/debug is missing '$want' -- not a real pilot debug surface"
       echo "$dbg" | head -40; exit 1 ;;
  esac
done

# /ready legitimately returns 503 here: the kubeconfig is unreachable, so cache sync
# never completes and the readiness flags never flip true. A real HTTP response (not
# "connection refused") is what proves the listener started, so check reachability
# instead of status code -- curl -f would wrongly fail on a correctly-behaving
# but not-cluster-connected control plane.
echo "checking the readiness endpoint answers (not refused)"
code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 http://127.0.0.1:8080/ready || true)"
case "$code" in
  ""|000) echo "/ready is not reachable (got '$code')"; docker logs "$NAME"; exit 1 ;;
  *) echo "/ready -> $code (503 expected without a cluster)" ;;
esac

# The first "waiting for sync" line is only emitted after the controller's 50th
# attempt (~4.5s in), so this has to poll rather than read the log once.
echo "checking the real config controller is attempting the API server"
for i in $(seq 1 30); do
  if docker logs "$NAME" 2>&1 | grep -q 'waiting for sync'; then
    break
  fi
  if [ "$i" = 30 ]; then
    echo "no 'waiting for sync' in the log: the config controller never started"
    docker logs "$NAME" 2>&1 | tail -30; exit 1
  fi
  sleep 1
done

# still running, i.e. it did not crash or exit while we probed it
[ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = "running" ] \
  || { echo "container is not running any more"; docker logs "$NAME" | tail -30; exit 1; }

echo "checking the ldflag-stamped version"
ver="$(docker run --rm --entrypoint /usr/bin/pilot-discovery "$IMAGE" version -s 2>/dev/null \
        | sed -n 's/^client version: //p' | tr -d '\r')"
echo "reported version: $ver"
case "$ver" in
  ""|unknown*) echo "version not stamped: '$ver'"; exit 1 ;;
esac
if [ -n "$EXPECT_VER" ]; then
  case "$ver" in
    "$EXPECT_VER"*) ;;
    *) echo "expected version '$EXPECT_VER', got '$ver'"; exit 1 ;;
  esac
fi

# must run as the nonroot istiod user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version: $ver, live /debug console + /ready, real config controller, nonroot user: $user)"
