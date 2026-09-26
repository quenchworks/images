#!/usr/bin/env bash
# Smoke test for a built WildFly image. Usage: test.sh <image-ref> [version]
# Boots it twice on a read-only rootfs with a tmpfs /tmp:
#   1. standalone.xml with a WAR whose JSP must compute "quench-42" server-side,
#      proving deployment, the scanner's writable markers and the JSP compiler;
#   2. standalone-microprofile.xml, the config that loads the OpenTelemetry modules
#      the recipe swapped to 1.62.0; the telemetry subsystem must come up.
# Each boot must be WFLYSRV0025 ("started"), not WFLYSRV0026 ("started with errors").
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-wildfly-smoke-$$"
WAR="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WAR"; }
trap cleanup EXIT
fail() { echo "$1"; docker logs "$NAME" 2>&1 | grep -vE '^\s+at ' | tail -40; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

python3 - "$WAR/quench.war" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], "w") as z:
    z.writestr("index.jsp", '<%= "quench-" + (6*7) %>\n')
PY
chmod 0755 "$WAR"; chmod 0644 "$WAR/quench.war"

boot() { # <extra args...>; leaves the container running
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker run -d --name "$NAME" --read-only --tmpfs /tmp:rw,mode=1777 \
    -v "$WAR/quench.war:/tmp/wildfly/deployments/quench.war:ro" \
    -p 127.0.0.1:8080:8080 "$IMAGE" "$@" >/dev/null
  for i in $(seq 1 120); do
    logs="$(docker logs "$NAME" 2>&1)"
    grep -qE 'WFLYSRV002[56]' <<<"$logs" && break
    [ "$i" = 120 ] && fail "WildFly did not finish booting"
    sleep 1
  done
  grep -q 'WFLYSRV0026' <<<"$logs" && fail "WildFly started WITH ERRORS"
  started="$(grep -oE 'WildFly [0-9.]+Final' <<<"$logs" | head -1)"
  echo "$started started: ${*:-standalone.xml}"
  [ -z "$WANT" ] || [ "$started" = "WildFly ${WANT}.Final" ] || fail "expected WildFly ${WANT}.Final"
  grep -q 'Http management interface listening on http://127.0.0.1:9990' <<<"$logs" \
    || fail "management is not bound to loopback only"
  body=""
  for i in $(seq 1 30); do
    body="$(curl -fsS http://127.0.0.1:8080/quench/ 2>/dev/null || true)"
    [ "$body" = "quench-42" ] && break
    sleep 1
  done
  [ "$body" = "quench-42" ] || fail "the deployed JSP did not answer quench-42 (got '$body')"
}

boot
code="$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/)"
[ "$code" = "200" ] || fail "welcome content not served at / (HTTP $code)"

boot -c standalone-microprofile.xml
grep -q 'WFLYMPTEL0001' <<<"$logs" || fail "the MicroProfile Telemetry subsystem did not activate"

echo "smoke test passed ($started, JSP deployed on both configs, telemetry up, nonroot user: $user)"
