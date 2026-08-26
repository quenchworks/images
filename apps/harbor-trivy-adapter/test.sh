#!/usr/bin/env bash
# Smoke test for the built Harbor Trivy scanner-adapter image.
# Usage: test.sh <image-ref>
#
# The image ships TWO Go binaries: scanner-trivy (the HTTP API adapter, the
# entrypoint) and trivy (the CLI it shells out to). The adapter normally needs
# Redis (job queue + result store) to be fully READY, but its LIVENESS probe
# GET /probe/healthy returns 200 without Redis -- which is exactly what we want
# to assert: the right binary boots, binds :8080, and serves /probe/healthy.
#
# Asserts:
#   - the bundled `trivy --version` prints the pinned TRIVYVERSION (0.70.0)
#   - /usr/bin/trivy is present, executable, and on PATH (LookPath resolves it)
#   - the scanner-trivy adapter boots and binds :8080, GET /probe/healthy -> 200
#   - the container runs as nonroot uid 1001 under a read-only rootfs with only
#     the trivy cache + reports dirs + /tmp writable
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-harbor-trivy-adapter-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# Derive the expected trivy version from build.conf's TRIVY map keyed on the app version,
# instead of hardcoding it. The hardcoded "0.70.0" was correct for 2.14.4 and silently wrong
# for 2.15.2, which bundles 0.72.0: the test failed on a perfectly good image. Same failure
# shape as the elasticsearch chart gate, which asserted a literal version and stalled a
# release for two weeks.
APPVER="${2:-}"
BC="$(dirname "$0")/build.conf"
if [ -n "$APPVER" ] && [ -f "$BC" ]; then
  # Scope to the TRIVY map line. build.conf also has a SCANNER map with the SAME keys, and
  # an unscoped match returns the scanner version (0.38.0 for 2.15.2) -- a plausible-looking
  # wrong answer that would make this assertion check nothing.
  WANT_TRIVY="$(grep -E 'declare -A TRIVY=' "$BC" \
                 | sed -n "s/.*\[$APPVER\]=\([0-9][0-9.]*\).*/\1/p" | head -1)"
fi
: "${WANT_TRIVY:?cannot determine the expected trivy version; pass the app version as \$2}"
echo "checking bundled trivy CLI version (expecting $WANT_TRIVY for app $APPVER)"
tv="$(docker run --rm --entrypoint /usr/bin/trivy "$IMAGE" --version 2>&1)"
echo "$tv"
# capture-then-match, no pipe into grep -q
grep -q "$WANT_TRIVY" <<<"$tv" \
  || { echo "bundled trivy is not $WANT_TRIVY"; exit 1; }

echo "checking /usr/bin/trivy is present, executable, and resolvable on PATH"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c '
  set -e
  test -x /usr/bin/trivy
  command -v trivy >/dev/null
  test -x /usr/bin/scanner-trivy
  echo "both binaries OK; trivy on PATH"
'

# Boot the adapter under a READ-ONLY rootfs. The only writable paths are the
# trivy cache + reports dirs and /tmp (tmpfs, owned by uid 1001). No Redis is
# provided: the adapter's liveness probe /probe/healthy must still answer 200.
# SCANNER_API_SERVER_ADDR defaults to :8080.
echo "booting scanner-trivy under read-only rootfs (writable cache/reports + /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw \
  --tmpfs /home/scanner/.cache:rw,uid=1001,gid=1001 \
  -p 18080:8080 \
  -e SCANNER_API_SERVER_ADDR=":8080" \
  -e SCANNER_LOG_LEVEL=info \
  "$IMAGE" >/dev/null

# Wait for the adapter to log that it's serving, or bail if it crashes.
ok=0
for i in $(seq 1 60); do
  logs="$(docker logs "$NAME" 2>&1 || true)"
  if echo "$logs" | grep -qiE 'Starting harbor-scanner-trivy|Starting API server|Listening'; then
    ok=1
    break
  fi
  if [ "$(docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null || echo false)" != "true" ]; then
    echo "container exited early:"; echo "$logs"; exit 1
  fi
  sleep 1
done
[ "$ok" = 1 ] || { echo "adapter did not report startup:"; docker logs "$NAME" 2>&1 | tail -50; exit 1; }
echo "adapter reports startup"

# Assert :8080 is listening (hex 8080 = 0x1F90), no external tools needed.
echo "checking the adapter is listening on :8080"
for i in $(seq 1 30); do
  if docker exec "$NAME" /bin/sh -c 'cat /proc/net/tcp /proc/net/tcp6 2>/dev/null' \
       | awk '{print $2}' | grep -qiE ':1F90$'; then
    listening=1
    break
  fi
  [ "$i" = 30 ] && { echo ":8080 not listening:"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
echo ":8080 listening -> OK"

# GET /probe/healthy must return 200 (liveness; independent of Redis). The
# hardened base ships a minimal busybox with no wget/nc applet and no /dev/tcp,
# so probe from the HOST against the published port (the test runner has curl).
echo "GET /probe/healthy must return 200 (probed from host on :18080)"
code=""
for i in $(seq 1 30); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/probe/healthy || true)"
  [ "$code" = "200" ] && break
  sleep 1
done
[ "$code" = "200" ] || { echo "/probe/healthy returned '$code', expected 200:"; docker logs "$NAME" | tail -30; exit 1; }
echo "/probe/healthy -> 200 OK"

# must run as the nonroot scanner user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot uid: $user, read-only rootfs, trivy $WANT_TRIVY bundled, /probe/healthy 200)"
