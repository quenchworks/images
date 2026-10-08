#!/usr/bin/env bash
# Smoke test for a built linkerd-cni image. Usage: test.sh <image-ref> [version]
# The plugin answers the CNI VERSION handshake, the repair controller reports its version, and
# the installer (the image's default command) really runs: it copies linkerd-cni into a stand-in
# host CNI bin dir. Wiring pods into the mesh needs a node: the chart's kind gate covers that.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
out="$(docker run --rm -e CNI_COMMAND=VERSION --entrypoint /opt/cni/bin/linkerd-cni "$IMAGE" </dev/null 2>&1)"
grep -q '"supportedVersions"' <<<"$out" || { echo "linkerd-cni: no CNI VERSION reply: $out"; exit 1; }
echo "linkerd-cni: ok"
out="$(docker run --rm --entrypoint /usr/lib/linkerd/linkerd-cni-repair-controller "$IMAGE" --version 2>&1)"
grep -q 'linkerd-cni-repair-controller' <<<"$out" || { echo "repair controller: $out"; exit 1; }
echo "repair controller: $out"
cmd="$(docker image inspect -f '{{join .Config.Cmd " "}}' "$IMAGE")"
out="$(docker run --rm --tmpfs /host/opt/cni/bin:exec --tmpfs /host/etc/cni/net.d --entrypoint /bin/sh "$IMAGE" \
  -c "$cmd >/tmp/install.log 2>&1 & for i in \$(seq 1 20); do [ -x /host/opt/cni/bin/linkerd-cni ] && break; sleep 1; done; \
      CNI_COMMAND=VERSION /host/opt/cni/bin/linkerd-cni </dev/null; echo; cat /tmp/install.log" 2>&1)"
grep -q '"supportedVersions"' <<<"$out" || { echo "installer did not install a working linkerd-cni: $out"; exit 1; }
echo "installer: copied linkerd-cni to the host bin dir"
echo "smoke test passed"
