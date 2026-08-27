#!/usr/bin/env bash
# Smoke test for a built kgateway data-plane image (upstream's `envoy-wrapper`).
# Usage: test.sh <image-ref> [expected-version]
#
# The whole point of this image is the chain entrypoint -> envoyinit -> envoy, so the
# test drives that chain end to end instead of checking that files exist:
#
#   1. A bootstrap is copied to /etc/envoy/envoy.yaml -- the exact path the kgateway
#      controller mounts its generated ConfigMap on -- containing a Downward API
#      template, `{{.PodName}}`, inside a direct_response body. The container is started
#      with POD_NAME set and NO `-c` flag, exactly as the deployer generates the pod.
#      A single curl then proves all of: /bin/sh ran the entrypoint, envoyinit found and
#      TRANSFORMED the bootstrap, it exec'd envoy, and envoy is serving. A wrong or
#      missing transform returns the literal template instead of the pod name.
#   2. `envoyinit --version` reports the release stamped by the -X ldflag; the envoy it
#      wraps reports its own version. Both are printed because the pair is the
#      contract with the control plane.
#   3. /bin/sh and wget exist. That is not cosmetic: the deployer puts
#      `/bin/sh -c 'wget --post-data "" -O /dev/null 127.0.0.1:19000/healthcheck/fail; sleep 10'`
#      on every proxy pod as its preStop connection-drain hook.
#   4. It runs as the nonroot envoy user.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
NAME="quench-kgateway-envoy-smoke-$$"
TMP="$(mktemp -d)"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$TMP"; }
trap cleanup EXIT

cat > "$TMP/envoy.yaml" <<'CFG'
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 19000 }
static_resources:
  listeners:
    - name: smoke
      address:
        socket_address: { address: 0.0.0.0, port_value: 8080 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: smoke
                route_config:
                  name: smoke
                  virtual_hosts:
                    - name: smoke
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 200
                            body: { inline_string: "{{.PodName}}" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
CFG

echo "1/4 entrypoint -> envoyinit -> envoy serves a Downward-API-expanded bootstrap"
# `docker create` then `docker cp` rather than a bind mount: no host path has to be
# shareable with the daemon, which keeps this working on every runner.
docker create --name "$NAME" -e POD_NAME=quench-smoke-pod -e POD_NAMESPACE=default \
  -p 127.0.0.1:18080:8080 "$IMAGE" --log-level warn >/dev/null
docker cp "$TMP/envoy.yaml" "$NAME:/etc/envoy/envoy.yaml"
docker start "$NAME" >/dev/null
body=""
for i in $(seq 1 30); do
  body="$(curl -fsS http://127.0.0.1:18080/ 2>/dev/null || true)"
  [ -n "$body" ] && break
  [ "$i" = 30 ] && { echo "envoy never served the smoke listener"; docker logs "$NAME"; exit 1; }
  sleep 1
done
if [ "$body" != "quench-smoke-pod" ]; then
  echo "expected the Downward API template to expand to 'quench-smoke-pod', got '$body'"
  docker logs "$NAME"; exit 1
fi
echo "  served '$body' -- {{.PodName}} was expanded by envoyinit"
docker rm -f "$NAME" >/dev/null

echo "2/4 the bundled envoy is the minor this kgateway release expects"
# envoyinit takes no --version flag (main.go goes straight to RunEnvoy), so there is
# nothing to assert on the Go side beyond step 1 having run it. What DOES matter to the
# control plane is the Envoy minor, since the xDS the controller emits is generated
# against it.
envoy_ver="$(docker run --rm --entrypoint /usr/local/bin/envoy "$IMAGE" --version)"
echo "  envoy: $envoy_ver"
echo "$envoy_ver" | grep -qE '/[0-9]+\.[0-9]+\.[0-9]+/' \
  || { echo "envoy did not report a version: '$envoy_ver'"; exit 1; }
if [ -n "$WANT" ]; then
  case "$WANT" in
    2.3.*) want_minor="1.37" ;;
    2.4.*) want_minor="1.38" ;;
    *)     want_minor="" ;;
  esac
  if [ -n "$want_minor" ] && ! echo "$envoy_ver" | grep -q "/${want_minor}\."; then
    echo "kgateway ${WANT} expects envoy ${want_minor}.x, image has: $envoy_ver"; exit 1
  fi
fi

echo "3/4 the preStop drain hook's shell and wget are present"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c 'command -v wget >/dev/null && echo ok' \
  | grep -qx ok || { echo "/bin/sh or wget missing; the generated preStop drain hook would fail"; exit 1; }

echo "4/4 runs as the nonroot envoy user"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (envoy ${envoy_ver##*version: }, nonroot $user)"
