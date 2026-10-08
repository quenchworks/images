#!/usr/bin/env bash
# Smoke test for a built calico-node image. Usage: test.sh <image-ref> [version]
# Checks that calico-node is our build of the version, that BIRD, the BPF objects and
# the runit services from Wolfi's package are present, and that the SPDX left for the
# Wolfi package lists no Go modules. The node agent itself needs a cluster: the chart's
# kind gate runs it as the CNI.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
out="$(docker run --rm --entrypoint /usr/bin/calico-node "$IMAGE" -v 2>&1)"
echo "$out"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$out" || { echo "expected calico-node v$WANT"; exit 1; }
docker run --rm --entrypoint /bin/bash "$IMAGE" -c '
set -e
for b in calico-node mountns bird bird6 birdcl start_runit ip ipset iptables-nft modprobe conntrack; do command -v $b >/dev/null || { echo "missing $b"; exit 1; }; done
bird --version 2>&1 | head -1
ls /usr/lib/calico/bpf/*.o >/dev/null
test -d /etc/service/available/felix && test -d /etc/calico/confd/templates
f=$(ls /var/lib/db/sbom/calico-node-*.spdx.json)
! grep -q "pkg:golang/" $f || { echo "Wolfi SPDX still lists Go modules: $f"; exit 1; }'
echo "smoke test passed"
