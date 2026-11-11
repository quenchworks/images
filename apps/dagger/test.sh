#!/usr/bin/env bash
# Smoke test for a built Dagger engine image. Usage: test.sh <image-ref> [version]
# Starts the engine privileged (as upstream documents), then from the CLI inside it:
# runs a container (runc, CNI), and initialises and calls a Go module (the bundled Go SDK
# runtime; its module downloads also prove DNS from inside the engine's network).
# Also unpacks that runtime's layer and gates it with Trivy, since the image scan does
# not look inside the engine's content store.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-dagger-smoke-$$"
WORK="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

ver="$(docker run --rm --entrypoint /usr/local/bin/dagger "$IMAGE" version)"
echo "cli: $ver"
[ -z "$WANT" ] || grep -qF "v$WANT" <<<"$ver" || { echo "expected v$WANT"; exit 1; }
grep -qF "image://ghcr.io/quenchworks/images/dagger:" <<<"$ver" || { echo "default runner is not this image"; exit 1; }

echo "gating the bundled Go SDK runtime layer"
docker create --name "$NAME-x" "$IMAGE" >/dev/null
docker cp "$NAME-x:/usr/local/share/dagger/content" "$WORK/content" >/dev/null
docker cp "$NAME-x:/etc/dagger/go-sdk-manifest-digest" "$WORK/digest" >/dev/null
docker rm "$NAME-x" >/dev/null
m="$(cut -d: -f2 "$WORK/digest")"
layer="$(jq -r '.layers[0].digest' "$WORK/content/blobs/sha256/$m" | cut -d: -f2)"
mkdir "$WORK/sdk" && tar -xzf "$WORK/content/blobs/sha256/$layer" -C "$WORK/sdk"
test -x "$WORK/sdk/usr/local/bin/codegen" && test -e "$WORK/sdk/usr/bin/go"
if command -v trivy >/dev/null; then
  trivy rootfs "$WORK/sdk" --exit-code 1 --ignore-unfixed --severity CRITICAL,HIGH,MEDIUM,LOW \
    --scanners vuln --detection-priority comprehensive --quiet
  echo "Go SDK runtime: 0 fixable CVEs"
elif [ -n "${CI:-}" ]; then
  echo "trivy is not on PATH in CI; the Go SDK runtime layer would go ungated"; exit 1
else
  echo "trivy not on PATH; Go SDK runtime layer not gated in this run"
fi

echo "starting the engine"
docker run -d --privileged --name "$NAME" "$IMAGE" >/dev/null
dx() { docker exec -e _EXPERIMENTAL_DAGGER_RUNNER_HOST=unix:///run/dagger/engine.sock -w /tmp/m "$NAME" "$@"; }
docker exec "$NAME" mkdir -p /tmp/m
for i in $(seq 1 90); do
  dx dagger --progress=plain core version >/dev/null 2>&1 && break
  [ "$i" = 90 ] && { echo "engine did not come up"; docker logs "$NAME" 2>&1 | tail -50; exit 1; }
  sleep 2
done
dx dagger --progress=plain core version

echo "running a container through the engine"
out="$(dx dagger --progress=plain -c 'container | from cgr.dev/chainguard/busybox:latest | with-exec -- echo quench | stdout')"
echo "$out"
grep -qx quench <<<"$out" || { docker logs "$NAME" 2>&1 | tail -50; exit 1; }

echo "initialising and calling a Go module"
dx dagger --progress=plain init --sdk=go --name=hello
out="$(dx dagger --progress=plain call container-echo --string-arg=quench stdout)"
echo "$out"
grep -qx quench <<<"$out" || { docker logs "$NAME" 2>&1 | tail -50; exit 1; }

echo "smoke test passed ($ver)"
