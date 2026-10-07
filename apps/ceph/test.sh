#!/usr/bin/env bash
# Smoke test for a built Ceph image. Usage: test.sh <image-ref> [version]
# Bootstraps a one-monitor cluster in the container (monmaptool, ceph-authtool, ceph-mon
# --mkfs as the ceph user), starts the mon, then asks it for status over the wire.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="ceph-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ver="$(docker run --rm "$IMAGE" --version | sed -n 's/^ceph version \([0-9.]*\).*/\1/p')"
echo "ceph $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }
for b in ceph-mon ceph-mgr ceph-osd ceph-mds radosgw rbd ceph-volume ceph-exporter ceph-bluestore-tool python3; do
  docker run --rm --entrypoint /bin/bash "$IMAGE" -c "command -v $b >/dev/null" || { echo "missing $b"; exit 1; }
done
echo "daemons and tools present"

docker run -d --name "$NAME" --entrypoint /bin/bash "$IMAGE" -c '
set -e
fsid=$(cat /proc/sys/kernel/random/uuid)
ip=$(hostname -i | cut -d" " -f1)
printf "[global]\nfsid = %s\nmon_initial_members = a\nmon_host = %s\nauth_allow_insecure_global_id_reclaim = false\n" "$fsid" "$ip" > /etc/ceph/ceph.conf
ceph-authtool --create-keyring /tmp/mon.keyring --gen-key -n mon. --cap mon "allow *"
ceph-authtool --create-keyring /etc/ceph/ceph.client.admin.keyring --gen-key -n client.admin \
  --cap mon "allow *" --cap osd "allow *" --cap mds "allow *" --cap mgr "allow *"
ceph-authtool /tmp/mon.keyring --import-keyring /etc/ceph/ceph.client.admin.keyring
chown ceph:ceph /tmp/mon.keyring
monmaptool --create --add a "$ip" --fsid "$fsid" /tmp/monmap
mkdir -p /var/lib/ceph/mon/ceph-a && chown -R ceph:ceph /var/lib/ceph/mon
ceph-mon --mkfs -i a --monmap /tmp/monmap --keyring /tmp/mon.keyring --setuser ceph --setgroup ceph
exec ceph-mon -f -i a --setuser ceph --setgroup ceph' >/dev/null
ok=0
for _ in $(seq 1 40); do
  st="$(docker exec "$NAME" ceph -s --connect-timeout 5 2>&1 || true)"
  grep -q 'mon: 1 daemons, quorum a' <<<"$st" && { ok=1; break; }
  sleep 2
done
[ "$ok" = 1 ] || { echo "mon never formed quorum: $st"; docker logs "$NAME" | tail -30; exit 1; }
echo "$st" | head -8
echo "smoke test passed"
