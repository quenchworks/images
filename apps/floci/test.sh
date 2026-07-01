#!/usr/bin/env bash
# Smoke test for a built floci image. Usage: test.sh <image-ref>
# Boots the Floci edge server (Quarkus JVM), waits for the health endpoint on
# :4566 to answer, then does a trivial in-process S3 check (create + list bucket)
# via the AWS REST API -- exercising one of the in-process services that run
# without a Docker socket (S3, DynamoDB, SQS, SNS, IAM). Runs entirely as the
# nonroot floci user (uid 1001); no docker.sock is mounted.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-floci-smoke-$$"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must run as the nonroot floci user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "nonroot user confirmed: $user"

echo "version (java -version on the runtime JRE):"
docker run --rm --entrypoint /usr/lib/jvm/java-25-openjdk/bin/java "$IMAGE" -version

echo "starting $IMAGE"
docker run -d --name "$NAME" -p 127.0.0.1:0:4566 "$IMAGE" >/dev/null
# resolve the host port docker mapped to the container's 4566 edge port
PORT="$(docker port "$NAME" 4566/tcp | head -1 | sed 's/.*://')"
[ -n "$PORT" ] || { echo "could not resolve mapped port"; docker logs "$NAME" | tail -50; exit 1; }
echo "container :4566 mapped to host :$PORT"
BASE="http://127.0.0.1:${PORT}"

# Floci boots the JVM in ~5-30s; poll /_floci/health for HTTP 200 up to ~120s.
ready=0; code=""
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}/_floci/health" || true)"
  if [ "$code" = "200" ]; then
    echo "floci health up after ~$((i*2))s (HTTP $code)"; ready=1; break
  fi
  sleep 2
done

if [ "$ready" != 1 ]; then
  echo "floci health endpoint did not respond in time (last HTTP '$code')"
  docker logs "$NAME" | tail -60; exit 1
fi

echo "health payload:"
curl -s "${BASE}/_floci/health" | head -c 400; echo

# Trivial in-process S3 check: create a bucket (PUT), then list buckets (GET).
# Uses the AWS REST API directly with dummy path-style creds (test/test), no SDK.
BUCKET="floci-smoke-$$"
put="$(curl -s -o /dev/null -w '%{http_code}' -X PUT "${BASE}/${BUCKET}" || true)"
echo "S3 create bucket ${BUCKET}: HTTP $put"
case "$put" in 200|201|204) ;; *) echo "S3 create bucket unexpected status $put"; docker logs "$NAME" | tail -40; exit 1 ;; esac

list="$(curl -s "${BASE}/" || true)"
echo "S3 list-buckets body (head):"
echo "$list" | head -c 400; echo
echo "$list" | grep -q "$BUCKET" || { echo "created bucket not present in list-buckets"; exit 1; }

echo "smoke test passed (nonroot user: $user, health HTTP $code, S3 in-process create+list OK)"
