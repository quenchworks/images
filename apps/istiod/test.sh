#!/usr/bin/env bash
# Smoke test for a built istiod image. Usage: test.sh <image-ref> [expected-version]
#
# istiod is a Kubernetes control plane: a full "does it manage a mesh" test needs
# a real cluster (that's the chart's kind install gate, not built in this task).
# But this must still prove something REAL, not just --version. The image's
# default command runs `pilot-discovery discovery --kubeconfig=/etc/istio/kubeconfig`
# against a baked-in, syntactically-valid but UNREACHABLE kubeconfig (see
# melange.yaml) -- a real, unstubbed pilot-discovery binary talking to a cluster
# it can't reach, same as any istiod pod before its RBAC/networking is wired up.
# (--registries=Mock was tried first and actually SIGSEGV'd: initServiceControllers
# unconditionally builds a serviceentry.Controller against a multicluster.Controller
# that is only non-nil once a kube client exists, verified by running it.) So we
# boot the actual discovery server and hit its real runtime HTTP endpoints:
#   :15014/version  -- pilot/pkg/bootstrap/monitoring.go writes version.Info.String()
#                       ("<version>-<git rev>-<status>"), so this reports the
#                       ldflag-stamped version from a live handler, not the CLI.
#   :15014/metrics  -- Prometheus text exposition including the `pilot_info` gauge
#                       (labeled with the same version string) and
#                       `istiod_uptime_seconds` (a live gauge derived from
#                       time.Since(serverStart)) -- proof the real xDS server
#                       booted and is running, not a stub that only parses flags.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"
NAME="quench-istiod-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE"
docker run -d --name "$NAME" -p 127.0.0.1:8080:8080 -p 127.0.0.1:15014:15014 "$IMAGE" >/dev/null

echo "waiting for the monitoring server (:15014) to come up"
for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:15014/version >/dev/null 2>&1; then
    break
  fi
  [ "$i" = 30 ] && { echo "istiod monitoring server did not come up"; docker logs "$NAME"; exit 1; }
  sleep 1
done

ver="$(curl -fsS http://127.0.0.1:15014/version)"
echo "reported version (live /version endpoint): $ver"
case "$ver" in
  ""|unknown*) echo "version not stamped: '$ver'"; exit 1 ;;
esac
if [ -n "$EXPECT_VER" ]; then
  case "$ver" in
    "$EXPECT_VER"*) ;;
    *) echo "expected version '$EXPECT_VER', got '$ver'"; exit 1 ;;
  esac
fi

echo "checking /metrics is a live Prometheus exporter reporting pilot_info + uptime"
metrics="$(curl -fsS http://127.0.0.1:15014/metrics)"
echo "$metrics" | grep -q '^pilot_info{' \
  || { echo "pilot_info gauge missing from /metrics"; echo "$metrics" | head -40; exit 1; }
echo "$metrics" | grep -q "pilot_info{version=\"${ver}\"}" \
  || { echo "pilot_info version label does not match /version"; echo "$metrics" | grep '^pilot_info'; exit 1; }
echo "$metrics" | grep -q '^istiod_uptime_seconds ' \
  || { echo "istiod_uptime_seconds gauge missing (server not actually running)"; exit 1; }

echo "checking the discovery/readiness HTTP server (:8080) is also live"
# /ready legitimately returns non-200 here: the kubeconfig is unreachable, so cache
# sync never completes and the injector/config-validation readiness flags never flip
# true. A real HTTP response (not "connection refused") is what proves the discovery
# server's own httpAddr listener started -- curl -f would wrongly fail the test on a
# correctly-behaving-but-not-cluster-connected control plane, so check reachability
# instead of status code.
if ! curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/ready | grep -qE '^[0-9]{3}$'; then
  echo "discovery HTTP server (:8080) is not reachable"; docker logs "$NAME"; exit 1
fi

# must run as the nonroot istiod user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (live version: $ver, pilot_info + uptime metrics present, nonroot user: $user)"
