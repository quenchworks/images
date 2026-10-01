#!/usr/bin/env bash
# Smoke test for a built vCluster syncer image. Usage: test.sh <image-ref> [version]
# The syncer needs a host cluster and the Kubernetes binaries to start, so this
# checks what the image can prove alone: the stamped version, the start command's
# flag parsing, and the nonroot user. The chart gate starts a real virtual cluster.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint /vcluster "$IMAGE" version)"
echo "vcluster: $ver"
case "$ver" in ""|dev|*0.0.0*) echo "version not stamped: '$ver'"; exit 1 ;; esac
if [ -n "$WANT" ] && [ "$ver" != "$WANT" ]; then echo "expected $WANT"; exit 1; fi

docker run --rm --entrypoint /vcluster "$IMAGE" start --help >/dev/null \
  || { echo "vcluster start --help failed"; exit 1; }

# the control plane the syncer execs from fixed paths, built from source in melange.yaml
for b in kube-apiserver kube-controller-manager; do
  kv="$(docker run --rm --entrypoint "/binaries/$b" "$IMAGE" --version)"
  [ "$kv" = "Kubernetes v1.36.4" ] || { echo "/binaries/$b reports '$kv'"; exit 1; }
done
docker run --rm --entrypoint /binaries/kine "$IMAGE" --version >/dev/null \
  || { echo "/binaries/kine failed"; exit 1; }

echo "smoke test passed ($ver, kube v1.36.4, nonroot user: $user)"
