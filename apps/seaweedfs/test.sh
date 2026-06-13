#!/usr/bin/env bash
# Smoke test for a built SeaweedFS image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the data dir (the read-only posture
# the chart ships), enables S3 auth via env, waits for the S3 /healthz, then
# does a real S3 roundtrip with the AWS CLI: make a bucket, put an object, get
# it back, and assert the content round-trips.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-seaweedfs-smoke-$$"
NET="quench-seaweedfs-net-$$"
AK="quenchadmin"
SK="quenchsecretkey123"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$NET" >/dev/null

echo "starting $IMAGE (read-only rootfs, tmpfs /data, S3 auth on)"
docker run -d --name "$NAME" --network "$NET" \
  --read-only \
  --tmpfs /data:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -e SEAWEED_S3_ACCESS_KEY="$AK" \
  -e SEAWEED_S3_SECRET_KEY="$SK" \
  -p 127.0.0.1:8333:8333 \
  "$IMAGE" >/dev/null

# wait for the S3 API to report healthy (/healthz is unauthenticated)
for i in $(seq 1 40); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:8333/healthz" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    echo "S3 API healthy after ${i}s"
    break
  fi
  [ "$i" = 40 ] && { echo "seaweedfs S3 did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# Real S3 roundtrip from a throwaway aws-cli container on the same network.
ENDPOINT="http://${NAME}:8333"
AWS="docker run --rm --network ${NET} \
  -e AWS_ACCESS_KEY_ID=${AK} -e AWS_SECRET_ACCESS_KEY=${SK} -e AWS_DEFAULT_REGION=us-east-1 \
  amazon/aws-cli --endpoint-url ${ENDPOINT}"

echo "creating bucket s3://gate"
# retry: the filer/volume can lag the S3 gateway's first healthz by a moment
for i in $(seq 1 15); do
  if $AWS s3 mb s3://gate >/dev/null 2>&1; then break; fi
  [ "$i" = 15 ] && { echo "could not create bucket"; docker logs "$NAME"; exit 1; }
  sleep 2
done

echo "putting and getting an object"
PAYLOAD="quenchworks-seaweedfs-roundtrip-$$"
docker run --rm --network "$NET" \
  -e AWS_ACCESS_KEY_ID="$AK" -e AWS_SECRET_ACCESS_KEY="$SK" -e AWS_DEFAULT_REGION=us-east-1 \
  --entrypoint sh amazon/aws-cli -c "
    set -e
    printf '%s' '${PAYLOAD}' > /tmp/obj.txt
    aws --endpoint-url ${ENDPOINT} s3 cp /tmp/obj.txt s3://gate/obj.txt
    aws --endpoint-url ${ENDPOINT} s3 cp s3://gate/obj.txt /tmp/got.txt
    got=\$(cat /tmp/got.txt)
    [ \"\$got\" = '${PAYLOAD}' ] || { echo \"roundtrip mismatch: got '\$got'\"; exit 1; }
    echo 'object roundtrip OK'
  "

echo "listing buckets"
$AWS s3 ls | grep -q gate || { echo "bucket not listed"; exit 1; }

# must run as the nonroot seaweedfs user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/weed version
echo "smoke test passed (nonroot user: $user, read-only rootfs, S3 bucket+object roundtrip OK)"
