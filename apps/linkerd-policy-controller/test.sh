#!/usr/bin/env bash
# Smoke test for a built linkerd-policy-controller image. Usage: test.sh <image-ref> [expected-version]
#
# Like linkerd-control-plane, a full "does it serve real policy" test needs a
# real cluster (the chart's kind gate, not built in this task). This proves
# the binary itself is real and running: clap arg parsing succeeded (a broken
# build would fail here with a usage error), the aws-lc-rs crypto provider
# installed successfully (main.rs `bail!`s if it doesn't -- see
# policy-controller/src/main.rs), and the admin server it starts via kubert
# is live and answering.
#
# Port numbers hit here (:9990 admin, :8090 grpc) are the assumptions recorded
# in apko.yaml -- --grpc-addr's default is confirmed from
# policy-controller/runtime/src/args.rs; --admin-addr's default is the kubert
# crate's own default and is NOT independently confirmed in this pass. If
# these are wrong, this test fails loudly (timeout) rather than false-passing.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
NAME="quench-linkerd-policy-controller-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# --default-opaque-ports is the ONE argument with no default. Everything else in
# policy-controller/runtime/src/args.rs either carries a default_value, is an
# Option, or is a bool flag; this one is a bare `#[clap(long)] String`, so
# starting with no arguments dies with "one or more required arguments were not
# provided" before the admin server binds. The value is upstream's own
# proxy.opaquePorts default from charts/linkerd-control-plane/values.yaml, not a
# number invented here.
echo "starting $IMAGE"
# --admission-controller-disabled too, or the next required argument is
# --server-tls-key: runtime/src/args.rs:128 reads
#   let server = if admission_controller_disabled { None } else { Some(server) }
# so disabling the admission controller is what drops the TLS server and its key
# and cert requirements. The boot test has no certificates to offer; the chart
# runs the admission controller with real ones.
docker run -d --name "$NAME" -p 127.0.0.1:9990:9990 -p 127.0.0.1:8090:8090 "$IMAGE" \
  --default-opaque-ports=25,587,3306,4444,5432,6379,9300,11211 \
  --admission-controller-disabled >/dev/null

echo "waiting for the admin server (:9990) to come up"
up=""
for i in $(seq 1 30); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:9990/ready 2>/dev/null || true)"
  if [ -n "$code" ] && [ "$code" != "000" ]; then
    up=1; break
  fi
  if ! docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null | grep -q true; then
    echo "container exited before the admin server came up"; docker logs "$NAME"; exit 1
  fi
  sleep 1
done
[ -n "$up" ] || { echo "admin server (:9990) never became reachable"; docker logs "$NAME"; exit 1; }

echo "checking /ready is reachable (a non-200 is expected: no real cluster to sync against)"
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9990/ready

echo "checking /metrics is a live Prometheus exporter"
metrics="$(curl -fsS http://127.0.0.1:9990/metrics)"
echo "$metrics" | grep -q '^# HELP ' \
  || { echo "no Prometheus exposition format in /metrics"; echo "$metrics" | head -20; exit 1; }

echo "checking the gRPC port (:8090) is at least accepting TCP connections"
if command -v nc >/dev/null 2>&1; then
  nc -z -w2 127.0.0.1 8090 || { echo "gRPC port :8090 refused connection"; docker logs "$NAME"; exit 1; }
else
  # /dev/tcp probe: bash builtin, no extra tooling required
  timeout 2 bash -c '</dev/tcp/127.0.0.1/8090' || { echo "gRPC port :8090 refused connection"; docker logs "$NAME"; exit 1; }
fi

# must run as the nonroot linkerd-policy-controller user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (admin server live, /metrics has real Prometheus output, gRPC port open, nonroot user: $user)"
