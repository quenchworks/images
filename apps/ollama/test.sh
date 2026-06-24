#!/usr/bin/env bash
# Smoke test for a built Ollama image. Usage: test.sh <image-ref>
# Runs with a READ-ONLY rootfs and a writable tmpfs at /models, waits for the
# inference server to answer GET /api/version, checks the reported version is
# stamped (not 0.0.0), exercises GET /api/tags, and confirms nonroot uid 1001.
# CPU-only: this gate does not pull or run a model (no GPU, no large download).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-ollama-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting $IMAGE (read-only rootfs, writable tmpfs /models)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /models:rw,mode=1777 \
  -p 127.0.0.1:11434:11434 \
  "$IMAGE" >/dev/null

# confirm nonroot uid 1001
UID_OUT="$(docker exec "$NAME" id -u)"
echo "runtime uid: $UID_OUT"
[ "$UID_OUT" = "1001" ] || { echo "FAIL: not running as uid 1001"; exit 1; }

# wait for GET /api/version -> {"version":"..."}
echo "waiting for /api/version"
ver=""
for i in $(seq 1 60); do
  ver="$(curl -fsS http://127.0.0.1:11434/api/version 2>/dev/null | sed -n 's/.*"version":"\([^"]*\)".*/\1/p' || true)"
  if [ -n "$ver" ]; then echo "/api/version ok after ${i}s"; break; fi
  if [ "$i" = 60 ]; then
    echo "FAIL: ollama did not answer /api/version"; docker logs "$NAME" 2>&1 | tail -60; exit 1
  fi
  sleep 1
done

echo "reported version: $ver"
case "$ver" in
  ""|0.0.0|*dev*) echo "FAIL: version not stamped: '$ver'"; exit 1 ;;
esac

# the model API must respond (empty model list on a fresh store -> {"models":[]})
echo "checking /api/tags:"
curl -fsS http://127.0.0.1:11434/api/tags >/dev/null \
  || { echo "FAIL: /api/tags did not respond"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

# in-image CLI must report the stamped version too
echo "in-image ollama --version check:"
CLI_VER="$(docker exec "$NAME" ollama --version 2>&1 | sed -n 's/.*client version is \([0-9][^ ]*\).*/\1/p')"
echo "  cli -> $CLI_VER"
[ "$CLI_VER" = "$ver" ] || { echo "FAIL: cli version '$CLI_VER' != server '$ver'"; exit 1; }

echo "PASS: ollama smoke test green (version $ver, nonroot user: $UID_OUT)"
