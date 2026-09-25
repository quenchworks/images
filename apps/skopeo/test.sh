#!/usr/bin/env bash
# Smoke test for a built skopeo image. Usage: test.sh <image-ref> [version]
# Real registry work: copy a multi-arch image (the QuenchWorks distribution image) from
# GHCR into a throwaway registry running that same image, then inspect it there and
# list its tags.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
SRC=ghcr.io/quenchworks/images/distribution:3.1.2
REG="skopeo-smoke-reg-$$"; NET="skopeo-smoke-$$"
cleanup() { docker rm -f "$REG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$ver"
[ -z "$WANT" ] || echo "$ver" | grep -q "version ${WANT}" || { echo "expected version $WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$REG" --network "$NET" --tmpfs /var/lib/registry:uid=1001,gid=1001 "$SRC" >/dev/null
run() { docker run --rm --network "$NET" --read-only --tmpfs /tmp --tmpfs /home/nonroot:uid=1001,gid=1001 "$IMAGE" "$@"; }
ok=0
for _ in $(seq 1 30); do
  run copy --all --dest-tls-verify=false "docker://$SRC" "docker://$REG:5000/smoke/distribution:v1" >/dev/null 2>&1 && { ok=1; break; }
  sleep 2
done
[ "$ok" = 1 ] || { echo "copy failed"; run copy --all --dest-tls-verify=false "docker://$SRC" "docker://$REG:5000/smoke/distribution:v1" || true; exit 1; }
src_digest="$(run inspect --raw "docker://$SRC" | sha256sum | cut -d' ' -f1)"
dst_digest="$(run inspect --raw --tls-verify=false "docker://$REG:5000/smoke/distribution:v1" | sha256sum | cut -d' ' -f1)"
[ "$src_digest" = "$dst_digest" ] || { echo "copied index differs ($src_digest vs $dst_digest)"; exit 1; }
run list-tags --tls-verify=false "docker://$REG:5000/smoke/distribution" | grep -q '"v1"' || { echo "tag not listed"; exit 1; }
echo "smoke test passed (skopeo ${WANT:-?}, uid $user, multi-arch copy byte-identical)"
