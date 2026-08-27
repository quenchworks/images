#!/usr/bin/env bash
# Smoke test for a built kgateway control-plane image.
# Usage: test.sh <image-ref> [expected-version]
#
# kgateway is a Kubernetes controller: it does nothing useful without an API server, so
# the usual "curl the app's port" pattern does not apply. This test asserts four things
# that DO hold in a bare `docker run`, none of which a broken build can fake:
#
#   1. `--version` reports the release stamped by the -X ldflag into
#      pkg/version.Version. An unstamped build prints "undefined".
#   2. The controller reaches its own setup and then FAILS, non-zero and quickly,
#      because there is no API server and no kubeconfig. That walks flag parsing,
#      settings decoding from the KGW_* environment and client construction; a binary
#      that cannot initialise at all dies before that. A hang is a failure here.
#   3. /usr/local/bin/envoy really is an executable Envoy that accepts
#      `--mode validate`. That is the exact fork pkg/validator performs for
#      `validation.level: strict` (defaultEnvoyPath is hard-coded), so this is a
#      functional test of a chart setting, not a file-exists check.
#   4. It runs as the nonroot kgateway user.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"

echo "1/4 --version is stamped by the ldflag"
ver_json="$(docker run --rm "$IMAGE" --version)"
echo "  $ver_json"
ver="$(printf '%s' "$ver_json" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')"
case "$ver" in
  ""|undefined|v0.0.0|*dev*) echo "version not stamped: '$ver'"; exit 1 ;;
esac
if [ -n "$WANT" ] && [ "$ver" != "v${WANT}" ]; then
  echo "expected version v${WANT}, got '$ver'"; exit 1
fi

echo "2/4 controller starts up, then fails non-zero without an API server"
out="$(timeout 120 docker run --rm "$IMAGE" 2>&1)" && rc=0 || rc=$?
printf '%s\n' "$out" | tail -5 | sed 's/^/  /'
[ "$rc" -ne 0 ]   || { echo "controller exited 0 with no cluster; expected a failure"; exit 1; }
[ "$rc" -ne 124 ] || { echo "controller hung instead of failing fast"; exit 1; }
echo "  controller exited $rc, as expected"

echo "3/4 /usr/local/bin/envoy validates a bootstrap (the strict-validation fork)"
envoy_ver="$(docker run --rm --entrypoint /usr/local/bin/envoy "$IMAGE" --version)"
echo "  $envoy_ver"
# The smallest bootstrap Envoy accepts, plus one deliberately bogus field in a second
# pass, so a validator that always says "ok" is caught too.
#
# JSON, not YAML, and that is not a style choice: `--config-path` picks its parser from
# the FILE EXTENSION, and /dev/fd/0 has none, so Envoy parses the stream as JSON and a
# YAML bootstrap fails with "Unable to parse JSON as proto". This is exactly the shape
# kgateway itself uses -- pkg/validator marshals the bootstrap to JSON and pipes it to
# `--config-path /dev/fd/0` -- so a YAML test here would have "passed" by asserting a
# rejection Envoy gives to any input.
ok_cfg='{"admin":{"address":{"socket_address":{"address":"127.0.0.1","port_value":19000}}}}'
printf '%s\n' "$ok_cfg" | docker run --rm -i --entrypoint /usr/local/bin/envoy "$IMAGE" \
  --mode validate --config-path /dev/fd/0 -l critical >/dev/null \
  || { echo "envoy rejected a valid bootstrap"; exit 1; }
if printf '%s\n' '{"admin":{"not_a_field":1}}' | docker run --rm -i --entrypoint /usr/local/bin/envoy \
     "$IMAGE" --mode validate --config-path /dev/fd/0 -l critical >/dev/null 2>&1; then
  echo "envoy accepted an invalid bootstrap; validation is not really running"; exit 1
fi
echo "  valid bootstrap accepted, invalid bootstrap rejected"

echo "4/4 runs as the nonroot kgateway user"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (kgateway $ver, nonroot $user)"
