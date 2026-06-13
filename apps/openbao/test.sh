#!/usr/bin/env bash
# Smoke test for a built OpenBao image. Usage: test.sh <image-ref>
#
# Proves the binary works end-to-end in DEV mode (auto-unsealed): /v1/sys/health 200,
# then a kv put/get roundtrip via the in-image `bao` CLI. ALSO sanity-checks the DEFAULT
# (non-dev) raft server boots to a sealed-but-listening state (health 501 not-initialized
# or 503 sealed -- both prove it's up). Read-only rootfs + writable tmpfs for data/config/tmp.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
DEV="quench-openbao-dev-$$"
SRV="quench-openbao-srv-$$"

cleanup() { docker rm -f "$DEV" "$SRV" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# --- must run as the nonroot openbao user (uid 1001) ---
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

############################################
# 1) DEV MODE: auto-unsealed, kv roundtrip
############################################
echo "== dev mode: starting $IMAGE (read-only rootfs + tmpfs) =="
docker run -d --name "$DEV" \
  --read-only \
  --tmpfs /openbao/data:rw,mode=1777 \
  --tmpfs /openbao/config:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -p 18200:8200 \
  -e BAO_DEV=1 \
  -e BAO_DEV_ROOT_TOKEN=root \
  "$IMAGE" >/dev/null

echo "waiting for dev server health (expect 200 init+unsealed)"
ok=0
for i in $(seq 1 30); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18200/v1/sys/health || true)"
  if [ "$code" = "200" ]; then ok=1; break; fi
  [ "$i" = 30 ] && { echo "dev health never hit 200 (last=$code)"; docker logs "$DEV"; exit 1; }
  sleep 1
done
[ "$ok" = 1 ] || exit 1
echo "dev health 200 OK"

echo "kv put/get roundtrip via in-image bao CLI"
docker exec -e BAO_ADDR=http://127.0.0.1:8200 -e BAO_TOKEN=root "$DEV" \
  bao kv put secret/quench value=hello >/dev/null
got="$(docker exec -e BAO_ADDR=http://127.0.0.1:8200 -e BAO_TOKEN=root "$DEV" \
  bao kv get -field=value secret/quench)"
[ "$got" = "hello" ] || { echo "kv roundtrip failed: got '$got'"; docker logs "$DEV"; exit 1; }
echo "kv roundtrip OK (value='$got')"

echo "dev version:"; docker exec "$DEV" bao version | head -1

############################################
# 2) DEFAULT (non-dev) raft server: sealed-but-listening
############################################
echo "== default raft server: expect sealed-but-listening (501/503) =="
docker run -d --name "$SRV" \
  --read-only \
  --tmpfs /openbao/data:rw,mode=1777 \
  --tmpfs /openbao/config:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -p 18300:8200 \
  "$IMAGE" >/dev/null

up=0
for i in $(seq 1 30); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18300/v1/sys/health || true)"
  # 501 = not initialized, 503 = sealed; either proves the listener is up.
  if [ "$code" = "501" ] || [ "$code" = "503" ]; then up=1; echo "raft node up, health=$code (sealed/uninitialized as expected)"; break; fi
  [ "$i" = 30 ] && { echo "raft node never reached listening state (last=$code)"; docker logs "$SRV"; exit 1; }
  sleep 1
done
[ "$up" = 1 ] || exit 1

echo "smoke test passed (nonroot user: $user, dev kv roundtrip OK, raft node listens sealed)"
