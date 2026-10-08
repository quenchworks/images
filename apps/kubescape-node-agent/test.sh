#!/usr/bin/env bash
# Smoke test for a built kubescape-node-agent image. Usage: test.sh <image-ref> [version]
# These are in-cluster daemons; outside a cluster they exit on missing config. Check each binary
# starts and prints its own output, not an exec failure (missing loader, wrong arch, bad mode).
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
run_bin() {
  local out rc=0
  out="$(timeout 20 docker run --rm --entrypoint "$1" "$IMAGE" --help 2>&1)" || rc=$?
  echo "$1 (exit $rc): $(head -c 300 <<<"$out")"
  [ "$rc" -ne 126 ] && [ "$rc" -ne 127 ] || { echo "$1 did not execute"; exit 1; }
  [ -n "$out" ] || { echo "$1 printed nothing"; exit 1; }
  if grep -qiE 'exec format error|no such file or directory.*exec|permission denied' <<<"$out"; then echo "$1 failed to exec"; exit 1; fi
}
run_bin /usr/bin/node-agent
run_bin /usr/bin/sbom-scanner
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
echo "user: $user"
[ "$user" = "0" ] || { echo "expected user 0"; exit 1; }
# node-agent opens tracers.tar relative to its working directory (kind crash 2026-10-08:
# "open /tracers.tar: no such file or directory"), so assert upstream's layout and that the
# archive is an OCI layout holding every gadget image node-agent asks for.
wd="$(docker inspect "$IMAGE" --format '{{.Config.WorkingDir}}')"
[ "$wd" = /root ] || { echo "expected WorkingDir /root, got '$wd'"; exit 1; }
T="$(mktemp -d)"; cid="$(docker create "$IMAGE")"
docker cp "$cid:/root/tracers.tar" "$T/tracers.tar" >/dev/null; docker cp "$cid:/root/.ig/config.yaml" "$T/config.yaml" >/dev/null
docker rm "$cid" >/dev/null
grep -q 'containerd-socketpath' "$T/config.yaml" || { echo "ig config missing"; exit 1; }
tar -xf "$T/tracers.tar" -C "$T" index.json
for ref in advise_seccomp trace_capabilities trace_dns trace_exec trace_open; do
  grep -q "gadget/$ref:v" "$T/index.json" || { echo "tracers.tar lacks $ref"; cat "$T/index.json"; exit 1; }
done
for ref in bpf exit fork hardlink http iouring_new iouring_old kmod network ptrace randomx ssh symlink unshare; do
  grep -qE "\"$ref:latest\"|/$ref:latest\"" "$T/index.json" || { echo "tracers.tar lacks $ref:latest"; cat "$T/index.json"; exit 1; }
done
echo "tracers.tar: $(du -h "$T/tracers.tar" | cut -f1), all 19 gadget images indexed"
rm -rf "$T"
echo "smoke test passed"
