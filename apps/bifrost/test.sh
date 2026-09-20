#!/usr/bin/env bash
# Smoke test for a built Bifrost image. Usage: test.sh <image-ref> <version>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <version>}"
VERSION="${2:?usage: test.sh <image-ref> <version>}"

# must run as the nonroot bifrost user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

name="bifrost-smoke-$$"
docker run -d --rm --name "$name" -p 18080:8080 "$IMAGE" >/dev/null
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

ok=0
for _ in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/ 2>/dev/null || true)"
  if [ "$code" = "200" ]; then ok=1; break; fi
  sleep 1
done
[ "$ok" = "1" ] || { echo "gateway did not serve / with HTTP 200"; docker logs "$name" 2>&1 | tail -30; exit 1; }

# The console is embedded in the binary with go:embed. Serving HTTP 200 on / is
# what proves the UI stage actually produced something; a gateway built with an
# empty ui/ directory still starts and still answers the API.
body="$(curl -s http://127.0.0.1:18080/ 2>/dev/null | head -c 2000)"
echo "$body" | grep -qi "<div id=\"root\"\|bifrost" \
  || { echo "/ did not return the embedded console markup"; exit 1; }

# The ldflag stamp must have reached the binary, so the image tag cannot disagree
# with what it actually runs.
ver="$(docker run --rm --entrypoint /usr/bin/bifrost "$IMAGE" -version 2>&1 | head -3 || true)"
echo "$ver" | grep -q "${VERSION}" || {
  # -version may not exist on every line; fall back to the API the server exposes.
  ver="$(curl -s http://127.0.0.1:18080/api/version 2>/dev/null || true)"
  echo "$ver" | grep -q "${VERSION}" \
    || { echo "version ${VERSION} not reported (got: $ver)"; exit 1; }
}

echo "smoke test passed (nonroot user: $user, console served HTTP 200, version ${VERSION})"
