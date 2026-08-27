#!/usr/bin/env bash
# Smoke test for a built NGINX Gateway Fabric control-plane image.
# Usage: test.sh <image-ref> [expected-version]
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"

# NOTE: there is no `gateway --version` and no `version` subcommand -- the root command
# has no cobra Version set, so `--version` is rejected as an unknown flag. The version
# ldflag is only observable from the controller's FIRST log line, which is emitted
# before any Kubernetes connection is attempted:
#   "Starting the NGINX Gateway Fabric control plane" {"version":"X.Y.Z", ...}
# So the version check and the fail-fast check are the same run. The image has no shell
# either (wolfi-baselayout + the binary), so nothing can be inspected from inside it.
out="$(docker run --rm \
  -e POD_NAMESPACE=default -e POD_NAME=t -e POD_UID=t -e INSTANCE_NAME=t -e IMAGE_NAME=t \
  "$IMAGE" controller \
  --gateway-ctlr-name=gateway.nginx.org/nginx-gateway-controller \
  --gatewayclass=nginx 2>&1 || true)"
printf '%s\n' "$out" | head -3

printf '%s' "$out" | grep -q 'Starting the NGINX Gateway Fabric control plane' \
  || { echo "the controller never reached its startup log"; exit 1; }

ver="$(printf '%s' "$out" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p' | head -1)"
echo "reported version: $ver"
case "$ver" in
  ""|0.0.0|*dev*|*unknown*) echo "version not stamped: '$ver'"; exit 1 ;;
esac
if [ -n "$WANT" ] && [ "$ver" != "$WANT" ]; then
  # Not cosmetic: the provisioner appends THIS string to the data-plane image repository
  # as the default tag, so a wrong stamp provisions the wrong nginx.
  echo "version mismatch: image says '$ver', expected '$WANT'"; exit 1
fi

# With no cluster reachable it must FAIL FAST rather than hang -- that is the shape of
# the crash-loop we would otherwise only see in a cluster.
# It must get PAST its own flag/ldflag parsing and fail on the missing cluster, not
# before it. A binary built with only -X main.version dies here instead, with
# `error parsing telemetry report period` -- see melange.yaml. Assert the failure is
# the cluster, so that regression cannot come back silently.
printf '%s' "$out" | grep -qi 'telemetry report period' \
  && { echo "binary is missing the telemetry ldflags -- it can never start"; exit 1; }
printf '%s' "$out" | grep -qiE 'kubeconfig|unable to|connection refused|cannot|no such host|KUBERNETES_SERVICE_HOST' \
  || { echo "the controller did not report the missing cluster config"; exit 1; }

# All three roles the chart uses must exist in this one binary: the controller, the cert
# generator (pre-install Job) and `initialize`, which the control plane injects as the
# init container of every nginx pod it provisions.
for sub in controller generate-certs initialize; do
  docker run --rm "$IMAGE" "$sub" --help >/dev/null \
    || { echo "missing subcommand: $sub"; exit 1; }
done
echo "subcommands present: controller, generate-certs, initialize"

# uid 101, not the house 1001: the provisioner hardcodes runAsUser 101 for the init
# container that runs THIS image.
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "101" ] || { echo "expected user 101, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user)"
