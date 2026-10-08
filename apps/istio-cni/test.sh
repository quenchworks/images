#!/usr/bin/env bash
# Smoke test for a built istio-cni image. Usage: test.sh <image-ref> [version]
# The plugin answers the CNI VERSION handshake, install-cni (found by name on PATH, as the chart
# runs it) reports the release, and every iptables variant the ambient agent picks between runs.
# Installing into a node and redirecting pod traffic needs a cluster: the chart's kind gate.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
out="$(docker run --rm -e CNI_COMMAND=VERSION --entrypoint /opt/cni/bin/istio-cni "$IMAGE" </dev/null 2>&1)"
grep -q '"supportedVersions"' <<<"$out" || { echo "istio-cni: no CNI VERSION reply: $out"; exit 1; }
echo "istio-cni: ok"
v="$(docker run --rm --entrypoint install-cni "$IMAGE" version 2>&1)"
echo "install-cni version: $v"
[ -z "$WANT" ] || grep -q "$WANT" <<<"$v" || { echo "expected install-cni $WANT"; exit 1; }
for b in iptables-legacy iptables-nft ip6tables-legacy ip6tables-nft iptables-legacy-save iptables-nft-save iptables-legacy-restore iptables-nft-restore ip6tables-legacy-restore ip6tables-nft-restore; do
  o="$(docker run --rm --entrypoint "$b" "$IMAGE" --version 2>&1)" || { echo "$b: $o"; exit 1; }
  echo "$b: $o"
done
echo "smoke test passed"
