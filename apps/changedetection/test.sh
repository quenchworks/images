#!/usr/bin/env bash
# Smoke test for a built changedetection.io image. Usage: test.sh <image-ref> [version]
# It runs on a read-only root with /datastore on a volume. The API key it generates into
# changedetection.json on first start drives the REST API: a wrong key is refused, a watch on a
# page served beside it is created and its snapshot holds the page text, and both survive a restart.
# The page is on a private Docker network, which changedetection.io refuses to fetch by default
# (SSRF guard), so the test container sets ALLOW_IANA_RESTRICTED_ADDRESSES=true. last_checked alone
# proves nothing: a BLOCKED fetch sets it too (the first version of this test passed that way).
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
TAG="cd-smoke-$$"
NET="$TAG-net"
URL=http://127.0.0.1:15000
CADDY=ghcr.io/quenchworks/images/caddy@sha256:6377c513134e78bd41fad0819c9d45830b1868398c1805a5b3ee8494f93072d7
cleanup() { docker rm -f "$TAG" "$TAG-web" >/dev/null 2>&1 || true; docker volume rm "$TAG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --entrypoint /opt/changedetection/bin/python "$IMAGE" -c 'import sys; sys.path.insert(0,"/app"); import changedetectionio; print(changedetectionio.__version__)')"
echo "changedetection.io $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

docker network create "$NET" >/dev/null
# the page to watch: our caddy image answering one line
docker run -d --name "$TAG-web" --network "$NET" --network-alias web --entrypoint /usr/bin/caddy "$CADDY" \
  respond --listen :8080 --body 'quench-watched-page' >/dev/null
docker volume create "$TAG" >/dev/null
start() {
  docker run -d --name "$TAG" --network "$NET" --read-only --tmpfs /tmp:uid=1001,gid=1001 -v "$TAG:/datastore" \
    -e ALLOW_IANA_RESTRICTED_ADDRESSES=true \
    -p 127.0.0.1:15000:5000 "$IMAGE" >/dev/null
  for i in $(seq 1 60); do curl -fsS "$URL/" >/dev/null 2>&1 && return 0
    [ "$i" = 60 ] && { echo "UI never answered"; docker logs "$TAG" 2>&1 | tail -30; exit 1; }; sleep 1; done
}
key() { docker run --rm -v "$TAG:/d" --entrypoint /opt/changedetection/bin/python "$IMAGE" -c \
  'import json; print(json.load(open("/d/changedetection.json"))["settings"]["application"]["api_access_token"])'; }
start
for i in $(seq 1 20); do k="$(key 2>/dev/null || true)"; [ -n "$k" ] && break; sleep 1; done
[ -n "$k" ] || { echo "no API key in the datastore"; exit 1; }
code="$(curl -s -o /dev/null -w '%{http_code}' -H 'x-api-key: wrong' "$URL/api/v1/watch")"
[ "$code" = 403 ] || [ "$code" = 401 ] || { echo "a wrong API key got HTTP $code"; exit 1; }
curl -fsS -H "x-api-key: $k" -H 'Content-Type: application/json' "$URL/api/v1/watch" \
  -d '{"url":"http://web:8080/","title":"quench-smoke-watch"}' >/dev/null
watches() { curl -fsS -H "x-api-key: $k" "$URL/api/v1/watch"; }
grep -q quench-smoke-watch <<<"$(watches)" || { echo "watch not listed: $(watches)"; exit 1; }
# the first check fetches the page: its snapshot must hold the page text
# by URL: first start also adds a default watch (news.ycombinator.com)
uuid="$(watches | jq -er 'first(to_entries[] | select(.value.url == "http://web:8080/") | .key)')"
snap=""
for i in $(seq 1 60); do
  snap="$(curl -fsS -H "x-api-key: $k" "$URL/api/v1/watch/$uuid/history/latest" 2>/dev/null || true)"
  grep -q quench-watched-page <<<"$snap" && break; sleep 2
done
grep -q quench-watched-page <<<"$snap" || { echo "the page was never fetched: '$snap'"; docker logs "$TAG" 2>&1 | tail -20; exit 1; }
echo "  wrong key refused ($code), watch created, page fetched into its snapshot"
docker rm -f "$TAG" >/dev/null
start
grep -q quench-smoke-watch <<<"$(watches)" || { echo "watch lost across a restart"; exit 1; }
echo "  watch and API key kept across a restart"
if docker logs "$TAG" 2>&1 | grep -qiE 'read-only file system|Traceback'; then echo "error in the logs"; docker logs "$TAG" 2>&1 | tail -20; exit 1; fi
echo "smoke test passed (changedetection.io $ver: API key, page text fetched into a snapshot, watch kept on /datastore, read-only root; user $user)"
