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
#
# Written to a file rather than piped into `head`: under `set -o pipefail`, head
# closing the pipe early makes curl exit 23 and takes the whole script with it.
# That is what failed CI on 2.1.1 while passing locally, because whether head
# closes first depends on the response size and timing.
body="$(mktemp)"
curl -s -o "$body" http://127.0.0.1:18080/ || true
grep -qi '<div id="root"\|bifrost' "$body" \
  || { echo "/ did not return the embedded console markup"; head -c 400 "$body"; exit 1; }

# The ldflag stamp must have reached the binary, so the image tag cannot disagree
# with what it actually runs. The binary has no -version flag (its flags are
# -port, -host, -app-dir, -log-level, -log-style) and no version endpoint; it
# prints main.Version in a banner on stdout at startup, so the running
# container's logs are where it is readable.
logs="$(mktemp)"
docker logs "$name" >"$logs" 2>&1 || true
grep -q "${VERSION}" "$logs" \
  || { echo "version ${VERSION} not found in the startup banner"; tail -25 "$logs"; exit 1; }

echo "smoke test passed (nonroot user: $user, console served HTTP 200, version ${VERSION})"
