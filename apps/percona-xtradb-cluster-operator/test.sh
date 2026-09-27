#!/usr/bin/env bash
# Smoke test for a built Percona XtraDB Cluster Operator image.
# Usage: test.sh <image-ref> [version]
# The operator needs a cluster to start, so this checks the parts the cluster's
# pods depend on: the PXC and HAProxy init scripts run as a nonroot user and
# install their scripts and helpers into a volume, and the manager starts and
# stops at the Kubernetes version lookup. The chart gate runs a real cluster.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

mkdir -p "$WORK/mysql" "$WORK/percona"; chmod 0777 "$WORK/mysql" "$WORK/percona"
docker run --rm --read-only -v "$WORK/mysql:/var/lib/mysql" --entrypoint /pxc-init-entrypoint.sh "$IMAGE" >/dev/null 2>&1 \
  || { echo "pxc-init-entrypoint.sh failed:"; docker run --rm --read-only -v "$WORK/mysql:/var/lib/mysql" --entrypoint /pxc-init-entrypoint.sh "$IMAGE" 2>&1 | tail -20; exit 1; }
for f in pxc-entrypoint.sh liveness-check.sh readiness-check.sh peer-list get-pxc-state mysql-state-monitor; do
  [ -x "$WORK/mysql/$f" ] || { echo "pxc init did not install $f"; ls -la "$WORK/mysql"; exit 1; }
done
echo "  pxc init installed its scripts and helpers"
docker run --rm --read-only -v "$WORK/percona:/opt/percona" --entrypoint /haproxy-init-entrypoint.sh "$IMAGE" >/dev/null 2>&1 \
  || { echo "haproxy-init-entrypoint.sh failed"; exit 1; }
[ -x "$WORK/percona/haproxy-entrypoint.sh" ] && [ -x "$WORK/percona/peer-list" ] \
  || { echo "haproxy init did not install its files"; ls -la "$WORK/percona"; exit 1; }
echo "  haproxy init installed its scripts"

# A kubeconfig pointing at a closed port: the manager must get as far as asking
# the API server for its version.
cat > "$WORK/kubeconfig" <<'KC'
apiVersion: v1
kind: Config
clusters: [{name: c, cluster: {server: "https://127.0.0.1:1"}}]
users: [{name: u, user: {token: x}}]
contexts: [{name: c, context: {cluster: c, user: u}}]
current-context: c
KC
chmod 0644 "$WORK/kubeconfig"
out="$(docker run --rm --read-only -v "$WORK/kubeconfig:/kubeconfig:ro" -e KUBECONFIG=/kubeconfig "$IMAGE" 2>&1 || true)"
grep -q "unable to define server version" <<<"$out" || { echo "manager did not reach the version lookup:"; tail -20 <<<"$out"; exit 1; }
echo "  manager started and reached the Kubernetes version lookup"

echo "smoke test passed (percona-xtradb-cluster-operator ${2:-?}: init scripts, manager start; nonroot user: $user)"
