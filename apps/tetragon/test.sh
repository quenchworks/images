#!/usr/bin/env bash
# Smoke test for a built Tetragon image. Usage: test.sh <image-ref> [version]
#
# Tetragon is an eBPF agent, so a real test has to load its BPF programs into
# the kernel. The test runs it the way the chart does: privileged, root, host
# PID namespace, bpffs mounted, events exported to a file. Then:
#   1. tetra version reports the release (the agent has no --version flag;
#      `tetra version --server` asks the running agent below).
#   2. The agent starts and `tetra status` (gRPC over its unix socket) reports
#      it running: BPF sensors loaded.
#   3. A process started in another container shows up in the gRPC event stream
#      (tetra getevents): the kprobes fire and the pipeline delivers.
# It needs a kernel with BTF (every current distro and Docker Desktop has one).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-tetragon-smoke-$$"
MARK="quench-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# The agent has no --version flag; tetra carries the same stamped version, and
# the agent's own startup log is checked for it below.
tver="$(docker run --rm --entrypoint /usr/bin/tetra "$IMAGE" version 2>&1 || true)"
echo "tetra: $tver"
grep -qE '[0-9]+\.[0-9]+\.[0-9]+' <<<"$tver" || { echo "tetra reported no version"; exit 1; }
if [ -n "${2:-}" ]; then grep -qF "${2}" <<<"$tver" || { echo "expected $2"; exit 1; }; fi

docker run -d --name "$NAME" --privileged --pid=host --user 0 \
  -v /sys/fs/bpf:/sys/fs/bpf --tmpfs /var/run/tetragon --tmpfs /tmp \
  "$IMAGE" --export-filename /tmp/events.log --log-level info >/dev/null

st=""
for i in $(seq 1 90); do
  st="$(docker exec "$NAME" /usr/bin/tetra status 2>&1 || true)"
  grep -qi 'running' <<<"$st" && break
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "agent died during startup:"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
  [ "$i" = 90 ] && { echo "tetra status never reported running: $st"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
  sleep 2
done
echo "tetra status: $st"
sv="$(docker exec "$NAME" /usr/bin/tetra version --server 2>&1 || true)"
echo "tetra version --server: $sv"
grep -qiE 'server version' <<<"$sv" || { echo "agent did not report its version over gRPC"; exit 1; }
if [ -n "${2:-}" ]; then grep -qF "${2}" <<<"$sv" || { echo "agent version is not $2"; exit 1; }; fi

# A process exec the agent must see through the host PID namespace, read back
# from the gRPC event stream (tetra getevents). Not from the export file:
# docker cp cannot read a tmpfs mount, which made that check read empty. The
# marker runs from the image under test, so no extra pull.
ev="$(mktemp)"
# getevents streams until killed (--timeout is only the connect timeout).
timeout 20 docker exec "$NAME" /usr/bin/tetra getevents -o compact > "$ev" 2>&1 &
gpid=$!
sleep 3
docker run --rm --entrypoint /usr/bin/tetra "$IMAGE" help "$MARK" >/dev/null 2>&1 || true
wait "$gpid" || true
grep -F "$MARK" "$ev" | head -3
grep -F "$MARK" "$ev" | grep -q 'process' || { echo "no process event for the marker process"; tail -20 "$ev"; rm -f "$ev"; exit 1; }
rm -f "$ev"
echo "process event streamed for the marker process"
echo "smoke test passed (BPF sensors loaded, exec events streamed)"
