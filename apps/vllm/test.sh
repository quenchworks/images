#!/usr/bin/env bash
# Smoke test for a built vLLM (CPU) image. Usage: test.sh <image-ref> [expected-version]
# vLLM is a GPU-runtime engine; QuenchWorks ships the CPU variant and the image gate is a
# CPU-ONLY smoke — we do NOT load a model or start the OpenAI server here (that needs
# downloaded weights + an avx512/avx2 host and is exercised by the chart, not the image
# gate). The smoke asserts: nonroot uid 1001, `vllm --version` reports <VER>+cpu, and
# `import vllm` succeeds (the engine's heavy import chain — torch(+cpu), transformers,
# the vllm C-extensions — all load cleanly).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

echo "vllm --version (no GPU, no model):"
VER_OUT="$(docker run --rm --entrypoint /usr/bin/vllm "$IMAGE" --version 2>/dev/null | tail -1)"
echo "  $VER_OUT"
echo "$VER_OUT" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\+cpu$' || { echo "FAIL: no <X.Y.Z>+cpu version line"; exit 1; }
if [ -n "$EXPECT_VER" ]; then
  echo "$VER_OUT" | grep -q "^${EXPECT_VER}+cpu$" \
    || { echo "FAIL: expected ${EXPECT_VER}+cpu"; exit 1; }
  echo "  version matches ${EXPECT_VER}+cpu"
fi

echo "import vllm (loads torch+cpu + transformers + the vllm C-extensions):"
IMP_OUT="$(docker run --rm --entrypoint /opt/vllm/venv/bin/python "$IMAGE" \
  -c 'import vllm; print("IMPORT_OK", vllm.__version__)' 2>/dev/null | tail -1)"
echo "  $IMP_OUT"
echo "$IMP_OUT" | grep -q '^IMPORT_OK ' || { echo "FAIL: import vllm did not succeed"; exit 1; }

echo "PASS: vllm CPU smoke green (uid 1001, vllm --version, import vllm)"
