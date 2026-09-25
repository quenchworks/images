#!/usr/bin/env bash
# Smoke test for a built oras image. Usage: test.sh <image-ref> [version]
# Real registry work: push a file as an OCI artifact to a throwaway registry (the
# QuenchWorks distribution image), then pull it back and compare it byte for byte.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
REG="oras-smoke-reg-$$"
NET="oras-smoke-$$"
cleanup() { docker rm -f "$REG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$ver"
[ -z "$WANT" ] || echo "$ver" | grep -q "Version: *${WANT}" || { echo "expected version $WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$REG" --network "$NET" --tmpfs /var/lib/registry:uid=1001,gid=1001 \
  ghcr.io/quenchworks/images/distribution:3.1.2 >/dev/null
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"; cleanup' EXIT
head -c 4096 /dev/urandom > "$WORK/blob.bin"
mkdir -p "$WORK/out" && chmod -R a+rwX "$WORK"

run() { docker run --rm --network "$NET" --read-only --tmpfs /tmp --tmpfs /home/nonroot:uid=1001,gid=1001 -v "$WORK:/work" -w /work "$IMAGE" "$@"; }
ok=0
for _ in $(seq 1 30); do
  run push --plain-http "$REG:5000/smoke/blob:v1" --artifact-type application/vnd.quench.smoke blob.bin >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "push failed"; run push --plain-http "$REG:5000/smoke/blob:v1" blob.bin || true; docker logs "$REG" 2>&1 | tail -20; exit 1; }
run manifest fetch --plain-http "$REG:5000/smoke/blob:v1" | grep -q 'application/vnd.quench.smoke' || { echo "artifact type not in manifest"; exit 1; }
run pull --plain-http "$REG:5000/smoke/blob:v1" -o out >/dev/null
cmp "$WORK/blob.bin" "$WORK/out/blob.bin" || { echo "pulled file differs"; exit 1; }
echo "smoke test passed (oras ${WANT:-?}, uid $user, push + pull round trip)"
