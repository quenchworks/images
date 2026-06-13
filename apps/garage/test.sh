#!/usr/bin/env bash
# Smoke test for a built Garage image. Usage: test.sh <image-ref>
# Runs read-only rootfs + writable tmpfs for the data dir (the read-only posture the
# chart ships), supplies the rpc_secret + admin_token + S3 creds via env, waits for
# the single-node layout bootstrap, then does a real S3 roundtrip with the AWS CLI:
# make a bucket, put an object, get it back, assert the content round-trips.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-garage-smoke-$$"
NET="quench-garage-net-$$"
AK="GK31c93ec1a0a0a0a0a0a0a0a0"      # Garage access keys are GK + exactly 24 hex chars
SK="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
RPC_SECRET="$(head -c32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
ADMIN_TOKEN="quenchadmintoken"
BUCKET="gate"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$NET" >/dev/null

echo "starting $IMAGE (read-only rootfs, tmpfs /data + /tmp, S3 creds + default bucket via env)"
docker run -d --name "$NAME" --network "$NET" \
  --read-only \
  --tmpfs /data:rw,mode=1777 \
  --tmpfs /tmp:rw,mode=1777 \
  -e GARAGE_RPC_SECRET="$RPC_SECRET" \
  -e GARAGE_ADMIN_TOKEN="$ADMIN_TOKEN" \
  -e GARAGE_S3_ACCESS_KEY="$AK" \
  -e GARAGE_S3_SECRET_KEY="$SK" \
  -e GARAGE_DEFAULT_BUCKET="$BUCKET" \
  -p 127.0.0.1:3900:3900 \
  -p 127.0.0.1:3903:3903 \
  "$IMAGE" >/dev/null

# wait for the admin API /health (unauthenticated 200 once the node is up + layout OK)
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:3903/health" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then
    echo "admin /health OK after ${i}s"
    break
  fi
  if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
    echo "container exited"; docker logs "$NAME"; exit 1
  fi
  [ "$i" = 60 ] && { echo "garage admin /health did not become healthy"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# Give the bootstrap (layout apply + key import + bucket create) a moment to finish.
echo "waiting for bootstrap to surface 'ready'"
for i in $(seq 1 30); do
  if docker logs "$NAME" 2>&1 | grep -q "garage: ready"; then break; fi
  [ "$i" = 30 ] && { echo "bootstrap did not complete"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# Real S3 roundtrip from a throwaway aws-cli container on the same network.
ENDPOINT="http://${NAME}:3900"
PAYLOAD="quenchworks-garage-roundtrip-$$"
echo "putting and getting an object via S3 (endpoint ${ENDPOINT}, region garage)"
for i in $(seq 1 15); do
  if docker run --rm --network "$NET" \
      -e AWS_ACCESS_KEY_ID="$AK" -e AWS_SECRET_ACCESS_KEY="$SK" -e AWS_DEFAULT_REGION=garage \
      --entrypoint sh amazon/aws-cli -c "
        set -e
        printf '%s' '${PAYLOAD}' > /tmp/obj.txt
        aws --endpoint-url ${ENDPOINT} s3 cp /tmp/obj.txt s3://${BUCKET}/obj.txt
        aws --endpoint-url ${ENDPOINT} s3 cp s3://${BUCKET}/obj.txt /tmp/got.txt
        got=\$(cat /tmp/got.txt)
        [ \"\$got\" = '${PAYLOAD}' ] || { echo \"roundtrip mismatch: got '\$got'\"; exit 1; }
        echo 'object roundtrip OK'
      "; then
    break
  fi
  [ "$i" = 15 ] && { echo "S3 roundtrip failed"; docker logs "$NAME"; exit 1; }
  sleep 2
done

# bucket should be listed by the garage CLI in-image
echo "verifying bucket via garage CLI"
docker exec "$NAME" sh -c 'GARAGE_CONFIG_FILE=/data/garage.toml garage bucket list' | grep -q "$BUCKET" \
  || { echo "bucket not listed by garage CLI"; docker logs "$NAME"; exit 1; }

# must run as the nonroot garage user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "version:"; docker exec "$NAME" /usr/bin/garage --version
echo "smoke test passed (nonroot user: $user, read-only rootfs, single-node layout bootstrap, S3 bucket+object roundtrip OK)"
