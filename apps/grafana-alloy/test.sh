#!/usr/bin/env bash
# Smoke test for a built Grafana Alloy image. Usage: test.sh <image-ref>
# Runs with a read-only rootfs (the posture the chart ships) plus a writable
# data dir + /tmp, with a trivial config, then asserts:
#   - `alloy --version` prints the pinned version (stamped, not "v0.0.0")
#   - GET :12345/-/ready -> 200 (Alloy HTTP server is up)
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-alloy-smoke-$$"
WORKDIR="$(mktemp -d)"
CONFIG="${WORKDIR}/config.alloy"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORKDIR"; }
trap cleanup EXIT

echo "alloy version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "v0.0.0")
echo "$out" | grep -qiE 'alloy, version v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# Trivial but valid Alloy config: a logging block (no external components needed).
cat > "$CONFIG" <<'EOF'
logging {
  level  = "info"
  format = "logfmt"
}
EOF
chmod 0644 "$CONFIG"

echo "starting $IMAGE (read-only rootfs; writable /tmp + data dir; :12345)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,uid=1001,gid=1001 \
  --tmpfs /var/lib/alloy/data:rw,uid=1001,gid=1001 \
  -v "$CONFIG:/etc/alloy/config.alloy:ro" \
  -p 127.0.0.1:12345:12345 \
  "$IMAGE" \
  run /etc/alloy/config.alloy \
  --server.http.listen-addr=0.0.0.0:12345 \
  --storage.path=/var/lib/alloy/data >/dev/null

# wait for the HTTP server /-/ready to report 200
for i in $(seq 1 60); do
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "http://127.0.0.1:12345/-/ready" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    break
  fi
  [ "$i" = 60 ] && { echo ":12345/-/ready did not become ready"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo ":12345/-/ready -> 200"

# must run as the nonroot alloy user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, :12345/-/ready OK)"
