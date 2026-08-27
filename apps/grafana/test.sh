#!/usr/bin/env bash
# Smoke test for a built Grafana image. Usage: test.sh <image-ref> [expected-version]
# Runs read-only rootfs + writable tmpfs for data/logs/provisioning + /tmp (the read-only
# posture the chart ships). Waits for /api/health, then asserts the DB is ok and (when a
# version is given as $2) that it matches, the login page serves HTML (frontend assets
# present), and basic-auth admin login reaches /api/org (the default org). Confirms
# nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"
NAME="quench-grafana-smoke-$$"
BASE="http://127.0.0.1:3000"
ADMIN_PW="quenchtest"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, tmpfs /var/lib/grafana + /var/log/grafana + /etc/grafana/provisioning + /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /var/lib/grafana:rw,mode=1777 \
  --tmpfs /var/log/grafana:rw,mode=1777 \
  --tmpfs /etc/grafana/provisioning:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -e GF_SECURITY_ADMIN_PASSWORD="$ADMIN_PW" \
  -p 127.0.0.1:3000:3000 \
  "$IMAGE" >/dev/null

# wait for the server to report healthy (SQLite init takes a few seconds on first boot)
health=""
for i in $(seq 1 60); do
  if health="$(curl -fsS "$BASE/api/health" 2>/dev/null)"; then
    break
  fi
  [ "$i" = 60 ] && { echo "grafana did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

echo "got /api/health: $health"
echo "$health" | grep -q '"database": *"ok"' || { echo "database not ok"; docker logs "$NAME"; exit 1; }
if [ -n "$EXPECT_VER" ]; then
  echo "$health" | grep -q "\"version\": *\"${EXPECT_VER}\"" \
    || { echo "version is not ${EXPECT_VER}"; docker logs "$NAME"; exit 1; }
  echo "  version matches ${EXPECT_VER}"
fi

echo "checking /login serves HTML (frontend assets present)"
login="$(curl -fsS "$BASE/login")"
echo "$login" | grep -qi "<html" || { echo "login page is not HTML"; exit 1; }
echo "$login" | grep -qi "grafana" || { echo "login page missing grafana markers"; exit 1; }

echo "checking basic-auth admin login reaches the default org"
org="$(curl -fsS -u "admin:${ADMIN_PW}" "$BASE/api/org")"
echo "got /api/org: $org"
echo "$org" | grep -q '"name"' || { echo "could not read default org with admin creds"; exit 1; }

echo "checking nonroot uid 1001"
uid="$(docker exec "$NAME" id -u)"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got $uid"; exit 1; }

echo "ALL GRAFANA SMOKE CHECKS PASSED"
