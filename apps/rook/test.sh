#!/usr/bin/env bash
# Smoke test for a built Rook image. Usage: test.sh <image-ref> [version]
# Checks the user, `rook version`, the ceph CLIs the operator shells out to, and the
# toolbox and monitoring files upstream's image ships. The operator itself is exercised
# by the chart's kind gate (it needs a cluster).
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "2016" ] || { echo "expected user 2016, got '$user'"; exit 1; }
out="$(docker run --rm "$IMAGE" version)"
echo "$out"
ver="$(sed -n 's/^rook: v\([0-9.]*\).*/\1/p' <<<"$out")"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected rook $WANT, got '$ver'"; exit 1; }
docker run --rm --entrypoint /bin/bash "$IMAGE" -c '
set -e
for b in ceph rados rbd radosgw-admin ceph-volume ip s5cmd toolbox.sh set-ceph-debug-level; do command -v $b >/dev/null || { echo "missing $b"; exit 1; }; done
ceph --version
test -d /etc/ceph-monitoring && test -f /etc/rook-external/create-external-cluster-resources.py'
echo "smoke test passed"
