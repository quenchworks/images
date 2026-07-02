#!/usr/bin/env bash
# Smoke test for a built Telegraf image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "telegraf version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$out"
# version must be stamped (built from the tag, not the default "unknown")
echo "$out" | grep -qiE 'Telegraf[[:space:]]+v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "version not stamped"; exit 1; }

# must run as the nonroot telegraf user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# functional smoke: one input -> --test (single gather cycle, prints to stdout).
# Keep the config beside this script so it lives on a bind-mount-shareable path
# (Docker Desktop refuses to mount /tmp).
cfg="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/.smoke.telegraf.conf"
trap 'rm -f "$cfg"' EXIT
cat > "$cfg" <<'CONF'
[agent]
  omit_hostname = true
[[inputs.mem]]
[[outputs.file]]
  files = ["stdout"]
CONF
echo "telegraf --test (inputs.mem):"
test_out="$(docker run --rm -v "$cfg:/etc/telegraf/telegraf.conf:ro" "$IMAGE" \
  --config /etc/telegraf/telegraf.conf --test 2>&1)"
echo "$test_out" | head -5
echo "$test_out" | grep -q '^> *mem ' \
  || { echo "expected a 'mem' metric line from --test"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
