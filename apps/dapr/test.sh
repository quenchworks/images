#!/usr/bin/env bash
# Smoke test for a built Dapr image. Usage: test.sh <image-ref> [version]
# Every binary reports the stamped version; daprd then runs in self-hosted mode on a
# read-only root and serves its health and metadata APIs. The chart gate runs the
# control plane and a sidecar-injected app in kind.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-dapr-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

for b in daprd operator placement sentry injector scheduler; do
  v="$(docker run --rm --entrypoint "/usr/bin/$b" "$IMAGE" --version 2>&1 | tail -1)"
  echo "$b: $v"
  case "$v" in ""|*edge*|*dev*) echo "$b version not stamped"; exit 1 ;; esac
  [ -z "$WANT" ] || grep -q "$WANT" <<<"$v" || { echo "$b: expected $WANT"; exit 1; }
done

docker run -d --name "$NAME" -p 127.0.0.1:13500:3500 --read-only --tmpfs /tmp "$IMAGE" \
  --app-id quench-smoke --mode standalone --dapr-http-port 3500 --resources-path /tmp \
  --dapr-listen-addresses 0.0.0.0 --log-level info >/dev/null
for i in $(seq 1 40); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:13500/v1.0/healthz/outbound || true)"
  [ "$code" = 204 ] && break
  [ "$i" = 40 ] && { echo "daprd never healthy (last $code)"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
meta="$(curl -fsS http://127.0.0.1:13500/v1.0/metadata)"
grep -q '"id":"quench-smoke"' <<<"$meta" || { echo "metadata lacks the app id: $meta"; exit 1; }
echo "  daprd self-hosted: healthy, metadata reports app quench-smoke"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (Dapr $v, nonroot user: $user, six binaries stamped, daprd self-hosted)"
