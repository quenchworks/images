#!/usr/bin/env bash
# Smoke test for a built harbor-jobservice image. Usage: test.sh <image-ref>
#
# jobservice needs Redis + a config file + core to actually run jobs (the
# /api/v1/stats health endpoint is the chart-agent's kind gate). Here we prove
# the BINARY is correct: it's harbor_jobservice with v2.14.4 stamped in, a
# static CGO-free ELF, starts and fails GRACEFULLY on missing config, nonroot.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
BIN=/usr/bin/harbor_jobservice

echo "checking the stamped release version is v2.14.4"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c "strings $BIN | grep -q 'v2.14.4'" \
  || { echo "release string v2.14.4 not found in $BIN"; exit 1; }

echo "checking the binary is a static ELF (no dynamic linker)"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c "
  set -e
  test -x $BIN
  if ldd $BIN 2>/dev/null | grep -q '=>'; then
    echo 'unexpected dynamic library dependencies:'; ldd $BIN; exit 1
  fi
  echo 'static OK'
"

echo "checking the binary starts and fails gracefully on missing config"
# jobservice with no -c config logs its init / a config error then exits -- NOT
# a linker crash.
out="$(docker run --rm "$IMAGE" 2>&1 || true)"
echo "$out" | head -5
echo "$out" | grep -qiE 'config|annotation parser|registered|jobservice' \
  || { echo "did not look like harbor_jobservice startup output"; echo "$out"; exit 1; }
echo "$out" | grep -qiE 'no such file or directory: /usr/bin/harbor_jobservice|exec format error' \
  && { echo "binary failed to exec (wrong arch / not static)"; exit 1; }
echo "starts + fails gracefully -> OK"

echo "checking nonroot uid 1001"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "harbor-jobservice smoke test passed (v2.14.4, static, nonroot uid $user)"
