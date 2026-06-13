#!/usr/bin/env bash
# Smoke test for a built harbor-exporter image. Usage: test.sh <image-ref>
#
# exporter needs a Postgres DB + core to scrape (the /metrics endpoint is the
# chart-agent's kind gate). Here we prove the BINARY is correct: it's
# harbor_exporter with v2.14.4 stamped in, a static CGO-free ELF, starts and
# fails GRACEFULLY on missing DB, nonroot.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
BIN=/usr/bin/harbor_exporter

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

echo "checking the binary starts and fails gracefully on missing DB/config"
# exporter with no DB config logs its init then fatals on InitDatabase -- NOT a
# linker crash.
out="$(docker run --rm "$IMAGE" 2>&1 || true)"
echo "$out" | head -5
echo "$out" | grep -qiE 'database|annotation parser|registered|exporter' \
  || { echo "did not look like harbor_exporter startup output"; echo "$out"; exit 1; }
echo "$out" | grep -qiE 'no such file or directory: /usr/bin/harbor_exporter|exec format error' \
  && { echo "binary failed to exec (wrong arch / not static)"; exit 1; }
echo "starts + fails gracefully -> OK"

echo "checking nonroot uid 1001"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "harbor-exporter smoke test passed (v2.14.4, static, nonroot uid $user)"
