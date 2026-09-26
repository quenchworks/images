#!/usr/bin/env bash
# Smoke test for a built NiFi image. Usage: test.sh <image-ref> [version]
# Sets single-user credentials, starts NiFi, then requires over HTTPS: the UI, a
# login token, the version from /flow/about, and PutS3Object from the aws NAR
# (the one NAR this image re-zips after swapping its wire jars).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-nifi-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" -p 127.0.0.1:8443:8443 --entrypoint /bin/sh "$IMAGE" -c \
  '/opt/nifi/bin/nifi.sh set-single-user-credentials admin quench-smoke-pass && exec /opt/nifi/bin/nifi.sh run' >/dev/null
U=https://localhost:8443
for i in $(seq 1 90); do
  [ "$(curl -sk -o /dev/null -w '%{http_code}' "$U/nifi/" || true)" = 200 ] && break
  [ "$i" = 90 ] && { echo "nifi did not come up"; docker logs "$NAME" | tail -40; exit 1; }
  sleep 4
done
tok="$(curl -fsk -X POST -d 'username=admin&password=quench-smoke-pass' "$U/nifi-api/access/token")"
[ -n "$tok" ] || { echo "login failed"; exit 1; }
about="$(curl -fsk -H "Authorization: Bearer $tok" "$U/nifi-api/flow/about")"
echo "$about" | head -c 200; echo
[ -z "$WANT" ] || grep -q "\"version\":\"$WANT\"" <<<"$about" || { echo "expected version $WANT"; exit 1; }
curl -fsk -H "Authorization: Bearer $tok" "$U/nifi-api/flow/processor-types?type=org.apache.nifi.processors.aws.s3.PutS3Object" \
  | grep -q '"artifact":"nifi-aws-nar"' || { echo "aws NAR did not load"; exit 1; }

echo "smoke test passed (nifi ${WANT:-?}: UI, login, aws NAR; nonroot user: $user)"
