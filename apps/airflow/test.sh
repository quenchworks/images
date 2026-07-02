#!/usr/bin/env bash
# Smoke test for a built Apache Airflow image. Usage: test.sh <image-ref> [expected-version]
#
# Airflow is a multi-component platform that needs a PostgreSQL metadata store (and a
# Redis broker for CeleryExecutor) to actually run its components — that full DB-backed
# boot (airflow db migrate + component readiness) is the CHART's kind-gate job. This
# image-level smoke only asserts, as nonroot uid 1001 on a read-only rootfs:
#   - `airflow version` reports the expected version (no DB required)
#   - `python -c "import airflow"` succeeds (the venv is importable)
#   - `airflow info` runs and reports the version (works without a live DB)
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

# Run read-only with tmpfs on the two writable paths the image declares, so we exercise
# the same read-only-rootfs posture the chart uses. mode=1777 makes the fresh tmpfs
# world-writable so uid 1001 can write airflow.cfg + logs (a docker --tmpfs defaults to
# root-owned 0755; the chart instead mounts a writable emptyDir owned by the pod fsGroup).
TMPFS=(--tmpfs /tmp:mode=1777 --tmpfs /opt/airflow:mode=1777)
DRUN=(docker run --rm --read-only "${TMPFS[@]}" "$IMAGE")

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "airflow version (no DB required):"
VER_OUT="$("${DRUN[@]}" version)"
echo "$VER_OUT" | sed 's/^/  /'
echo "$VER_OUT" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "FAIL: no version line"; exit 1; }
if [ -n "$EXPECT_VER" ]; then
  echo "$VER_OUT" | grep -qx "${EXPECT_VER}" \
    || { echo "FAIL: expected airflow version ${EXPECT_VER}"; exit 1; }
  echo "  version matches ${EXPECT_VER}"
fi

echo "python -c 'import airflow' (venv importable):"
IMP_OUT="$(docker run --rm --read-only "${TMPFS[@]}" \
  --entrypoint /opt/airflow-venv/bin/python "$IMAGE" -c 'import airflow; print(airflow.__version__)')"
echo "  import airflow -> ${IMP_OUT}"
echo "$IMP_OUT" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "FAIL: import airflow failed"; exit 1; }

echo "airflow info (runs without a live DB):"
if "${DRUN[@]}" info >/tmp/airflow-info.$$ 2>&1; then
  grep -E 'version|Apache Airflow' /tmp/airflow-info.$$ | head -5 | sed 's/^/  /' || true
  echo "  airflow info OK"
else
  echo "FAIL: airflow info exited nonzero"; tail -30 /tmp/airflow-info.$$; rm -f /tmp/airflow-info.$$; exit 1
fi
rm -f /tmp/airflow-info.$$

echo "PASS: airflow image smoke green (version + import + info, nonroot uid 1001, read-only rootfs)"
