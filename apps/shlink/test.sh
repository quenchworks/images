#!/usr/bin/env bash
# Smoke test for a built Shlink image. Usage: test.sh <image-ref> [version]
# Shlink runs on RoadRunner on a read-only root with data/ (SQLite by default) on a volume. Through
# the REST API with the INITIAL_API_KEY: a wrong key is refused, a short URL is created and its
# redirect answers 302 to the long URL, and the short URL survives a restart.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="shlink-smoke-$$"
URL=http://127.0.0.1:18085
KEY="smoke-key-$$-0123456789abcdef"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
rr="$(docker run --rm --entrypoint /etc/shlink/bin/rr "$IMAGE" -v)"; echo "$rr"
docker volume create "$NAME" >/dev/null
start() {
  docker run -d --name "$NAME" --read-only --tmpfs /tmp:uid=1001,gid=1001,mode=1777 -v "$NAME:/etc/shlink/data" \
    -e DEFAULT_DOMAIN=s.example -e IS_HTTPS_ENABLED=false -e INITIAL_API_KEY="$KEY" \
    -p 127.0.0.1:18085:8080 "$IMAGE" >/dev/null
  for i in $(seq 1 90); do curl -fsS "$URL/rest/health" >/dev/null 2>&1 && return 0
    [ "$i" = 90 ] && { echo "/rest/health never answered"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }; sleep 1; done
}
start
health="$(curl -fsS "$URL/rest/health")"; echo "$health"
ver="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["version"])' "$health")"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT, got $ver"; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' -H 'X-Api-Key: wrong' "$URL/rest/v3/short-urls")"
[ "$code" = 401 ] || { echo "a wrong API key got HTTP $code"; exit 1; }
curl -fsS -H "X-Api-Key: $KEY" -H 'Content-Type: application/json' "$URL/rest/v3/short-urls" \
  -d '{"longUrl":"https://quench-works.com/smoke","customSlug":"qsmoke"}' >/dev/null
redirect() { curl -s -o /dev/null -w '%{http_code} %{redirect_url}' -H 'Host: s.example' "$URL/qsmoke"; }
r="$(redirect)"; echo "  redirect: $r"
[ "$r" = "302 https://quench-works.com/smoke" ] || { echo "the short URL does not redirect"; exit 1; }
echo "  wrong key refused, short URL created and redirecting"
docker rm -f "$NAME" >/dev/null
start
[ "$(redirect)" = "302 https://quench-works.com/smoke" ] || { echo "short URL lost across a restart"; exit 1; }
echo "  short URL kept across a restart"
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|PHP Fatal'; then echo "error in the logs"; docker logs "$NAME" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (shlink $ver on RoadRunner: API key, short URL redirect kept on the data volume, read-only root; user $user)"
