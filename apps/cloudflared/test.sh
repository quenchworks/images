#!/usr/bin/env bash
# Smoke test for a built cloudflared image. Usage: test.sh <image-ref> [version]
# A tunnel needs a Cloudflare account, so this checks what runs without one: the stamped version,
# and a tunnel started with a token and no network reaching its metrics server (/metrics answers,
# and cloudflared_tunnel build info carries the version) while it retries the edge.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="cloudflared-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE")"
echo "$v"
[ -z "$WANT" ] || grep -qF "cloudflared version $WANT " <<<"$v" || { echo "expected $WANT"; exit 1; }

# a syntactically valid token (base64 JSON with account, tunnel id, secret); the edge is never
# reached because the container has no route out
tok="$(printf '{"a":"00000000000000000000000000000000","t":"11111111-1111-1111-1111-111111111111","s":"%s"}' "$(printf 'smoke-secret-0123456789abcdef0123' | base64)" | base64 -w0)"
docker run -d --name "$NAME" --read-only -p 127.0.0.1:12000:2000 "$IMAGE" \
  tunnel --no-autoupdate --metrics 0.0.0.0:2000 run --token "$tok" >/dev/null
ok=0
for _ in $(seq 1 30); do
  m="$(curl -fsS http://127.0.0.1:12000/metrics 2>/dev/null || true)"
  grep -q '^build_info{' <<<"$m" && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "metrics server never answered"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
[ -z "$WANT" ] || grep -q "^build_info{.*version=\"$WANT\"" <<<"$m" || { echo "build_info lacks $WANT"; grep '^build_info' <<<"$m"; exit 1; }
echo "smoke test passed: version, tunnel metrics server with build info, user 1001"
