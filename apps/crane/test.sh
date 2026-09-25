#!/usr/bin/env bash
# Smoke test for a built crane image. Usage: test.sh <image-ref> [version]
# Real registry work: build an image from a local layer with `crane append`, push it to
# a throwaway registry (the QuenchWorks distribution image), then read its digest and
# export its filesystem back.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
REG="crane-smoke-reg-$$"
NET="crane-smoke-$$"
cleanup() { docker rm -f "$REG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" version 2>&1)"
echo "crane $ver"
[ -z "$WANT" ] || echo "$ver" | grep -q "$WANT" || { echo "expected version $WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$REG" --network "$NET" --tmpfs /var/lib/registry:uid=1001,gid=1001 \
  ghcr.io/quenchworks/images/distribution:3.1.2 >/dev/null
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"; cleanup' EXIT
mkdir -p "$WORK/root/etc" && echo "quenchworks-crane-smoke" > "$WORK/root/etc/smoke.txt"
tar -C "$WORK/root" -cf "$WORK/layer.tar" etc
chmod -R a+rwX "$WORK"

run() { docker run --rm --network "$NET" --read-only --tmpfs /tmp --tmpfs /home/nonroot:uid=1001,gid=1001 -v "$WORK:/work" -w /work "$IMAGE" "$@"; }
ok=0
for _ in $(seq 1 30); do
  run append --insecure -f layer.tar -t "$REG:5000/smoke/img:v1" >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "append failed"; run append --insecure -f layer.tar -t "$REG:5000/smoke/img:v1" || true; exit 1; }
digest="$(run digest --insecure "$REG:5000/smoke/img:v1")"
echo "digest $digest"
echo "$digest" | grep -qE '^sha256:[0-9a-f]{64}$' || { echo "bad digest"; exit 1; }
run catalog --insecure "$REG:5000" | grep -qx 'smoke/img' || { echo "repository not in catalog"; exit 1; }
run export --insecure "$REG:5000/smoke/img:v1" out.tar
tar -xOf "$WORK/out.tar" etc/smoke.txt | grep -qx 'quenchworks-crane-smoke' || { echo "exported file missing"; exit 1; }
echo "smoke test passed (crane ${WANT:-?}, uid $user, append + digest + export round trip)"
