#!/usr/bin/env bash
# Smoke test for a built nessie image. Usage: test.sh <image-ref> [version]
# Real catalog work on a read-only root, on the RocksDB store (its JNI library needs
# libstdc++): Nessie must report ready, return its config, create a branch off main and
# list it back.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="nessie-smoke-$$"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp \
  -e NESSIE_VERSION_STORE_TYPE=ROCKSDB -e NESSIE_VERSION_STORE_PERSIST_ROCKS_DATABASE_PATH=/tmp/rocksdb \
  -p 127.0.0.1:19120:19120 -p 127.0.0.1:19000:9000 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 90); do
  curl -fsS http://127.0.0.1:19000/q/health/ready 2>/dev/null | grep -q '"status": *"UP"' && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "nessie never became ready"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

API=http://127.0.0.1:19120/api/v2
cfg="$(curl -fsS "$API/config")"
echo "$cfg" | grep -q '"defaultBranch" *: *"main"' || { echo "unexpected config: $cfg"; exit 1; }
[ -z "$WANT" ] || docker logs "$NAME" 2>&1 | grep -q "$WANT" || { echo "version $WANT not in the startup log"; exit 1; }
hash="$(curl -fsS "$API/trees/main" | sed -n 's/.*"hash" *: *"\([0-9a-f]*\)".*/\1/p')"
[ -n "$hash" ] || { echo "no hash for main"; exit 1; }
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d "{\"type\":\"BRANCH\",\"name\":\"main\",\"hash\":\"$hash\"}" \
  "$API/trees?name=quench-smoke&type=BRANCH" >/dev/null
curl -fsS "$API/trees" | grep -q '"name" *: *"quench-smoke"' || { echo "branch not listed"; exit 1; }
if docker logs "$NAME" 2>&1 | grep -E ' ERROR |Exception'; then echo "nessie logged errors"; exit 1; fi
echo "smoke test passed (nessie ${WANT:-?}, uid $user, branch created and listed)"
