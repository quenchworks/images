#!/usr/bin/env bash
# Smoke test for a built prometheus-config-reloader image. Usage: test.sh <image-ref> [version]
# The reloader renders Prometheus' config with environment variables substituted (what the operator
# relies on for $(POD_NAME) in shard labels) and serves /healthz. Run it on a read-only root against
# a config with a placeholder and require the rendered file to carry the value.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="reloader-smoke-$$"
cleanup() { docker rm -f "$NAME" "$NAME-cfg" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version 2>&1)"; echo "$v" | head -1
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }
docker volume create "$NAME" >/dev/null
# the image has no shell: write and read the volume with our busybox image
BB=ghcr.io/quenchworks/images/busybox@sha256:041302c8da3a2e182ecc2eac3c56c4f47a13c03cc284d255489c0be741893b64
printf 'global:\n  external_labels:\n    replica: $(POD_NAME)\n' | docker run --rm -i -u 0 -v "$NAME:/c" --entrypoint /bin/sh "$BB" -c 'cat > /c/in.yaml && chown 1001 /c'
docker run -d --name "$NAME" --read-only -e POD_NAME=quench-smoke-0 -v "$NAME:/c" -p 127.0.0.1:18085:8080 "$IMAGE" \
  --listen-address=:8080 --config-file=/c/in.yaml --config-envsubst-file=/c/out.yaml --reload-url=http://127.0.0.1:9/-/reload >/dev/null
for i in $(seq 1 30); do curl -fsS http://127.0.0.1:18085/healthz >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { echo "/healthz never answered"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }; sleep 1; done
out=""
for i in $(seq 1 20); do out="$(docker run --rm -v "$NAME:/c" --entrypoint /bin/cat "$BB" /c/out.yaml 2>/dev/null || true)"; grep -q 'quench-smoke-0' <<<"$out" && break; sleep 1; done
grep -q 'replica: quench-smoke-0' <<<"$out" || { echo "config not rendered: $out"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
echo "  rendered config carries replica: quench-smoke-0; /healthz ok"
echo "smoke test passed (prometheus-config-reloader ${WANT:-?}: envsubst render, health endpoint, read-only root; user $user)"
