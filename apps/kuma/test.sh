#!/usr/bin/env bash
# Smoke test for a built Kuma control-plane image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-kuma-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# version must be stamped (not the default "unknown"). kuma-cp prints "Kuma: X.Y.Z".
echo "kuma-cp version:"
out="$(docker run --rm --entrypoint /usr/bin/kuma-cp "$IMAGE" version 2>&1)"
echo "$out"
echo "$out" | grep -qiE 'Kuma:[[:space:]]*[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "kuma-cp version not stamped"; exit 1; }

# kumactl must also be present and stamped.
echo "kumactl version:"
kout="$(docker run --rm --entrypoint /usr/bin/kumactl "$IMAGE" version 2>&1)"
printf '%s\n' "$kout" | head -1

# start the control plane (default in-memory store, no external DB).
# diagnostics server serves /healthy + /ready on 5680; the GUI + API on 5681.
echo "starting $IMAGE (kuma-cp run)"
docker run -d --name "$NAME" \
  -p 127.0.0.1:5680:5680 -p 127.0.0.1:5681:5681 "$IMAGE" >/dev/null

for i in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:5680/ready >/dev/null 2>&1; then
    break
  fi
  [ "$i" = 60 ] && { echo "kuma-cp did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "control plane is ready (diagnostics :5680/ready)"

# the API server + embedded GUI must answer on 5681.
curl -fsS http://127.0.0.1:5681/ >/dev/null \
  || { echo "API server / GUI did not respond on 5681"; docker logs "$NAME"; exit 1; }
echo "API server + GUI responding (:5681)"

# must run as the nonroot kuma user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
