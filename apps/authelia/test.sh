#!/usr/bin/env bash
# Smoke test for a built Authelia image. Usage: test.sh <image-ref> [expected-version]
# The runtime image is distroless (no shell / coreutils), so process-level checks go
# through the authelia binary itself and the image config. Authelia needs a config file
# + a storage backend (sqlite/postgres) + optional redis to actually `serve`, so a full
# boot is exercised by the chart's kind-install gate, not here. For the IMAGE we confirm:
# the image runs as nonroot uid 1001, and `authelia --version` (needs no config/DB)
# prints the expected build. Presence of the binary is asserted at melange build time
# (melange.yaml test pipeline).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "authelia --version (no config/DB required), run read-only as the image user:"
VER_OUT="$(docker run --rm --read-only "$IMAGE" --version)"
echo "$VER_OUT" | sed 's/^/  /'
echo "$VER_OUT" | grep -q '^authelia version v' || { echo "FAIL: no 'authelia version' line"; exit 1; }

if [ -n "$EXPECT_VER" ]; then
  echo "$VER_OUT" | grep -q "^authelia version v${EXPECT_VER}\$" \
    || { echo "FAIL: expected authelia version v${EXPECT_VER}"; exit 1; }
  echo "  version matches v${EXPECT_VER}"
fi

echo "PASS: authelia smoke test green"

# Real boot check: the from-source binary must serve HTTP (this catches missing
# go:embed assets — e.g. public_html/api/index.html — that --version never touches).
echo "boot check: start with a minimal config and probe /api/health ..."
TDIR="$(mktemp -d "$(pwd)/smoke.XXXXXX")"
cat > "$TDIR/configuration.yml" <<'CFG'
theme: light
server:
  address: tcp://0.0.0.0:9091
log:
  level: info
identity_validation:
  reset_password:
    jwt_secret: insecure_jwt_secret_for_smoke_test_only_0123456789abcdef
authentication_backend:
  file:
    path: /config/users.yml
access_control:
  default_policy: one_factor
session:
  secret: insecure_session_secret_for_smoke_test_only_0123456789
  cookies:
    - domain: example.com
      authelia_url: https://auth.example.com
storage:
  encryption_key: insecure_storage_encryption_key_for_smoke_test_only
  local:
    path: /config/db.sqlite3
notifier:
  filesystem:
    filename: /config/notification.txt
CFG
cat > "$TDIR/users.yml" <<'USR'
users:
  smoke:
    disabled: false
    displayname: Smoke Test
    password: "$argon2id$v=19$m=65536,t=3,p=4$c21va2V0ZXN0c2FsdA$V1DVfUXlKV0KqmGmpsu4Ba1PjB47Vh4hTDqvbleyLE0"
    email: smoke@example.com
USR
chmod -R a+rwX "$TDIR"
CID="$(docker run -d --read-only -v "$TDIR:/config" "$IMAGE" --config /config/configuration.yml)"
ok=0
for i in $(seq 1 30); do
  if docker exec "$CID" /usr/bin/authelia healthcheck 2>/dev/null; then ok=1; break; fi
  # fallback probe if healthcheck subcommand unavailable
  if docker logs "$CID" 2>&1 | grep -q "Startup complete"; then ok=1; break; fi
  if [ "$(docker inspect -f '{{.State.Running}}' "$CID")" != "true" ]; then break; fi
  sleep 2
done
docker logs "$CID" 2>&1 | tail -5
docker rm -f "$CID" >/dev/null 2>&1 || true
rm -rf "$TDIR"
[ "$ok" = "1" ] || { echo "FAIL: authelia did not reach a healthy started state"; exit 1; }
echo "  boot check passed (server started, assets loaded)"
