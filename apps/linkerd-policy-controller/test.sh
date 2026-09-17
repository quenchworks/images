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

# WHAT THIS TEST CAN AND CANNOT PROVE, as of edge-26.8.4.
#
# It used to drive the admin server on :9990. That is not reachable without a
# real cluster. Even with the admission controller disabled, the runtime's
# leader-election lease is fetched before the admin server serves, retried
# against the API server, and the process exits when it never answers:
#   linkerd_policy_controller_runtime::lease: Failed to fetch deployment,
#   retrying in 1s... ConnectError(... 127.0.0.1:6443 ... Connection refused)
# No flag combination avoids it; the lease is not optional.
#
# So this is a BOOT test: it proves the binary runs, is linked, parses its
# arguments, reaches its OWN runtime code, and then fails for the ONE documented
# reason rather than a link error, a missing library, or a panic. Functional
# proof lives in the chart's kind install gate, which has a real API server.
sleep 8
logs="$(docker logs "$NAME" 2>&1)"

echo "checking the binary reached its own runtime"
echo "$logs" | grep -q 'linkerd_policy_controller_runtime' \
  || { echo "never reached the policy-controller runtime:"; echo "$logs"; exit 1; }

echo "checking it failed for the expected reason, not something else"
echo "$logs" | grep -q 'Connection refused' \
  || { echo "did not fail the documented way; read the log before trusting this image:"; echo "$logs"; exit 1; }

echo "checking nothing panicked"
if echo "$logs" | grep -qE "panic|SIGSEGV|no such file or directory|cannot execute|error while loading shared"; then
  echo "the binary did not start cleanly:"; echo "$logs"; exit 1
fi

# must run as the nonroot linkerd-policy-controller user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "boot test passed (binary runs, reached its own runtime, failed only on the unreachable API server, nonroot user: $user)"
