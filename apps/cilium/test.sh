#!/usr/bin/env bash
# Smoke test for the Cilium agent image. Usage: test.sh <image-ref> [version]
# A real datapath needs a node (kind with the default CNI off); the chart gate does that.
# Here: the binaries report the version, the image's own clang compiles a BPF object with
# the shipped headers (the agent does this on the node at runtime), and the CNI pieces the
# install-cni init container copies to the host are present and answer.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
run() { docker run --rm --entrypoint /bin/bash "$IMAGE" -c "$1"; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
# Root by design: upstream's chart runs the agent and its host-writing init containers as the
# image user (see apko.yaml).
[ "$user" = "0" ] || { echo "expected user 0, got '$user'"; exit 1; }
out="$(run 'cilium-agent --version; hubble version; cilium-dbg --help >/dev/null && echo cilium-dbg ok')"
echo "$out"
[ -z "$WANT" ] || grep -q "$WANT" <<<"$out" || { echo "expected version $WANT"; exit 1; }
out="$(run 'printf "%s\n" "#include <bpf/ctx/skb.h>" "#include <bpf/api.h>" "__section(\"tc\") int t(struct __ctx_buff *ctx) { return 0; }" "BPF_LICENSE(\"Dual BSD/GPL\");" > /tmp/t.c &&
  clang -O2 --target=bpf -nostdinc -I/var/lib/cilium/bpf/include -I/var/lib/cilium/bpf -c /tmp/t.c -o /tmp/t.o && llc --version | head -2 && bpftool version | head -1 && ls -l /tmp/t.o')"
echo "$out"
grep -q 't.o' <<<"$out" || { echo "clang did not compile a BPF object"; exit 1; }
out="$(run 'CNI_COMMAND=VERSION /cni/loopback </dev/null; echo; ls /install-plugin.sh /init-container.sh /cni-uninstall.sh; ip -V; iptables --version; command -v ipset kmod')"
echo "$out"
grep -q 'cniVersion' <<<"$out" || { echo "CNI loopback did not answer"; exit 1; }
echo "smoke test passed (cilium ${WANT:-?}: binaries, runtime BPF compile, CNI and network tools)"
