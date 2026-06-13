#!/usr/bin/env bash
# Smoke test for a built RustFS image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the data dir (the read-only posture the
# chart ships), supplies the root S3 creds via env, waits for the unauthenticated
# /health endpoint on the S3 port, then does a real S3 roundtrip with the AWS CLI:
# make a bucket, put an object, get it back, assert the content round-trips.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-rustfs-smoke-$$"
NET="quench-rustfs-net-$$"
AK="quenchadmin"
SK="quenchsecret0123456789"
BUCKET="gate"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$NET" >/dev/null

echo "starting $IMAGE (read-only rootfs, tmpfs /data + /tmp, root S3 creds via env)"
docker run -d --name "$NAME" --network "$NET" \
  --read-only \
  --tmpfs /data:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -e RUSTFS_ACCESS_KEY="$AK" \
  -e RUSTFS_SECRET_KEY="$SK" \
  -p 127.0.0.1:9000:9000 \
  -p 127.0.0.1:9001:9001 \
  "$IMAGE" >/dev/null

# wait for the unauthenticated /health (200 once the S3 server is up).
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:9000/health" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    echo "S3 /health OK after ${i}s"
    break
  fi
  if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
    echo "container exited"; docker logs "$NAME"; exit 1
  fi
  [ "$i" = 60 ] && { echo "rustfs /health did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# Real S3 roundtrip from a throwaway aws-cli container on the same network.
ENDPOINT="http://${NAME}:9000"
PAYLOAD="quenchworks-rustfs-roundtrip-$$"
echo "creating bucket + putting/getting an object via S3 (endpoint ${ENDPOINT})"
for i in $(seq 1 15); do
  if docker run --rm --network "$NET" \
      -e AWS_ACCESS_KEY_ID="$AK" -e AWS_SECRET_ACCESS_KEY="$SK" -e AWS_DEFAULT_REGION=us-east-1 \
      --entrypoint sh amazon/aws-cli -c "
        set -e
        aws --endpoint-url ${ENDPOINT} s3 mb s3://${BUCKET} 2>/dev/null || true
        printf '%s' '${PAYLOAD}' > /tmp/obj.txt
        aws --endpoint-url ${ENDPOINT} s3 cp /tmp/obj.txt s3://${BUCKET}/obj.txt
        aws --endpoint-url ${ENDPOINT} s3 cp s3://${BUCKET}/obj.txt /tmp/got.txt
        got=\$(cat /tmp/got.txt)
        [ \"\$got\" = '${PAYLOAD}' ] || { echo \"roundtrip mismatch: got '\$got'\"; exit 1; }
        aws --endpoint-url ${ENDPOINT} s3 ls s3://${BUCKET}/ | grep -q obj.txt
        echo 'object roundtrip OK'
      "; then
    break
  fi
  [ "$i" = 15 ] && { echo "S3 roundtrip failed"; docker logs "$NAME"; exit 1; }
  sleep 2
done

# must run as the nonroot rustfs user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/rustfs --version | head -1
echo "smoke test passed (nonroot user: $user, read-only rootfs, S3 /health, S3 bucket+object roundtrip OK)"
