#!/usr/bin/env bash
# Boot test for a built kfp-frontend image. Usage: test.sh <image-ref> [version]
# Starts the UI server, fetches the client and the server's own healthz, and runs the two
# dependency patches (query-string, minio's stream-json) inside the image.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got $user"; exit 1; }
out="$(docker run --rm --entrypoint /usr/bin/node "$IMAGE" --input-type=module -e '
  import { Client } from "minio";
  import { createRequire } from "module";
  const require = createRequire("/server/package.json");
  const qs = require("query-string");
  if (qs.parse("a=%20b").a !== " b") throw new Error("query-string decode");
  if (typeof Client !== "function") throw new Error("minio import");
  console.log("deps ok");
' 2>&1 || true)"
grep -q "deps ok" <<<"$out" || { echo "patched deps fail in the image: $out"; exit 1; }
NAME="kfp-frontend-boot-$$"
docker run -d --name "$NAME" -p 127.0.0.1::3000 "$IMAGE" >/dev/null
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT
port="$(docker port "$NAME" 3000 | head -1 | sed 's/.*://')"
ok=""
for _ in $(seq 1 60); do
  body="$(curl -fsS "http://127.0.0.1:$port/" 2>/dev/null || true)"
  if grep -q 'id="root"' <<<"$body"; then ok=1; break; fi
  sleep 1
done
[ -n "$ok" ] || { echo "UI did not serve index.html"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
hz="$(curl -fsS "http://127.0.0.1:$port/apis/v1beta1/healthz" 2>/dev/null || true)"
echo "healthz: $hz"
[ -z "$WANT" ] || grep -q "$WANT" <<<"$hz" || { echo "healthz does not report $WANT"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
echo "smoke test passed"
