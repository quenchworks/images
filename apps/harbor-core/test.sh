#!/usr/bin/env bash
# Smoke test for a built harbor-core image. Usage: test.sh <image-ref>
#
# harbor-core needs a full Harbor cluster (Postgres, Redis, the registry,
# config) to actually serve. Without that we can't exercise /api/v2.0/ping --
# that's the chart-agent's kind gate. Here we prove the BINARY is correct:
#   - it's the harbor_core binary with the pinned release v2.14.4 stamped in
#   - it's a static (CGO-free) ELF -- starts with no dynamic-linker error
#   - it starts and fails GRACEFULLY on missing config (right binary, runs)
#   - the container runs as nonroot uid 1001
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
BIN=/usr/bin/harbor_core

echo "checking the stamped release version is v2.14.4"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c "strings $BIN | grep -q 'v2.14.4'" \
  || { echo "release string v2.14.4 not found in $BIN"; exit 1; }

echo "checking the binary is a static ELF (no dynamic linker)"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c "
  set -e
  test -x $BIN
  # ldd on a static binary reports 'not a dynamic executable' / errors out.
  if ldd $BIN 2>/dev/null | grep -q '=>'; then
    echo 'unexpected dynamic library dependencies:'; ldd $BIN; exit 1
  fi
  echo 'static OK'
"

echo "checking the binary starts and fails gracefully on missing config"
# harbor_core with no config logs its init then exits non-zero -- NOT a linker
# crash (exit 127 / 'No such file or directory' from the loader). Capture output.
out="$(docker run --rm "$IMAGE" 2>&1 || true)"
echo "$out" | head -5
echo "$out" | grep -qiE 'app.conf|config|annotation parser|registered' \
  || { echo "did not look like harbor_core startup output"; echo "$out"; exit 1; }
echo "$out" | grep -qiE 'no such file or directory: /usr/bin/harbor_core|exec format error' \
  && { echo "binary failed to exec (wrong arch / not static)"; exit 1; }
echo "starts + fails gracefully -> OK"

echo "checking nonroot uid 1001"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "harbor-core smoke test passed (v2.14.4, static, nonroot uid $user)"
