#!/usr/bin/env bash
# Smoke test for a built calico-cni image. Usage: test.sh <image-ref> [version]
# Runs the CNI binaries' VERSION command (the CNI spec's version handshake) and checks
# the calico plugin reports the release. The installer itself needs a node (it writes
# host paths): the Calico chart's kind gate runs it.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
for b in calico calico-ipam bandwidth host-local loopback portmap tuning flannel; do
  out="$(docker run --rm -e CNI_COMMAND=VERSION --entrypoint "/opt/cni/bin/$b" "$IMAGE" </dev/null 2>&1)"
  grep -q '"supportedVersions"' <<<"$out" || { echo "$b: no CNI VERSION reply: $out"; exit 1; }
  echo "$b: ok"
done
v="$(docker run --rm --entrypoint /opt/cni/bin/calico "$IMAGE" -v 2>&1 || true)"
echo "calico -v: $v"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$v" || { echo "expected calico v$WANT"; exit 1; }
echo "smoke test passed"
