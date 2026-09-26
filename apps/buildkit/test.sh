#!/usr/bin/env bash
# Smoke test for a built rootless BuildKit image. Usage: test.sh <image-ref> [version]
# Runs buildkitd the way upstream documents the rootless image (seccomp and
# AppArmor unconfined, no process sandbox), then builds a FROM scratch Dockerfile
# through buildctl with the built-in dockerfile frontend, so nothing is pulled.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-buildkit-smoke-$$"
WORK="$(mktemp -d)"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint /usr/bin/buildkitd "$IMAGE" --version)"
echo "buildkitd: $ver"
case "$ver" in *" v0."*|*" v1."*) ;; *) echo "version not stamped"; exit 1 ;; esac
if [ -n "$WANT" ]; then
  case "$ver" in *" v${WANT} "*|*" v${WANT}") ;; *) echo "expected v${WANT}"; exit 1 ;; esac
fi

echo "starting rootless buildkitd"
docker run -d --name "$NAME" \
  --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  "$IMAGE" --oci-worker-no-process-sandbox >/dev/null

for i in $(seq 1 60); do
  if docker exec "$NAME" buildctl debug workers >/dev/null 2>&1; then break; fi
  [ "$i" = 60 ] && { echo "buildkitd did not come up"; docker logs "$NAME"; exit 1; }
  sleep 1
done
docker exec "$NAME" buildctl debug workers

echo "building a FROM scratch Dockerfile"
docker exec "$NAME" sh -c 'mkdir -p /home/nonroot/.local/tmp/ctx && cd /home/nonroot/.local/tmp/ctx \
  && echo quench > hello.txt \
  && printf "FROM scratch\nCOPY hello.txt /hello.txt\n" > Dockerfile'
docker exec "$NAME" buildctl build --frontend dockerfile.v0 \
  --local context=/home/nonroot/.local/tmp/ctx --local dockerfile=/home/nonroot/.local/tmp/ctx \
  --output type=tar,dest=- > "$WORK/out.tar" \
  || { echo "build failed"; docker logs "$NAME"; exit 1; }
tar -xOf "$WORK/out.tar" hello.txt | grep -qx quench \
  || { echo "build output missing hello.txt"; tar -tf "$WORK/out.tar"; exit 1; }

echo "smoke test passed ($ver, nonroot user: $user)"
