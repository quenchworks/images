#!/usr/bin/env bash
# Smoke test for a built linkding image. Usage: test.sh <image-ref> [version]
# linkding starts on a read-only root with data on a volume, creates the initial superuser from
# the environment, answers /health, and the REST API creates a bookmark and lists it back.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="linkding-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm --entrypoint /bin/cat "$IMAGE" /etc/linkding/version.txt)"
echo "linkding $v"
[ -z "$WANT" ] || [ "$v" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

docker volume create "$NAME" >/dev/null
docker run -d --name "$NAME" --read-only --tmpfs /tmp -v "$NAME:/etc/linkding/data" -p 127.0.0.1:19090:9090 \
  -e LD_SUPERUSER_NAME=smoke -e LD_SUPERUSER_PASSWORD=smoke-pass-123 "$IMAGE" >/dev/null
for _ in $(seq 1 60); do curl -fsS http://127.0.0.1:19090/health >/dev/null 2>&1 && break; sleep 2; done
curl -fsS http://127.0.0.1:19090/health || { echo "/health never answered"; docker logs "$NAME" 2>&1 | tail -25; exit 1; }
echo
# an API token for the superuser, from manage.py in the running container
# linkding's own ApiToken model (bookmarks.api.auth.LinkdingTokenAuthentication), not DRF's Token
tok="$(docker exec "$NAME" python manage.py shell -c "from django.contrib.auth.models import User; from bookmarks.models import ApiToken; print('TOKEN', ApiToken.objects.create(user=User.objects.get(username='smoke'), name='smoke').key)" | awk '$1=="TOKEN"{print $2}')"
[ -n "$tok" ] || { echo "no API token"; exit 1; }
curl -fsS -X POST http://127.0.0.1:19090/api/bookmarks/ -H "Authorization: Token $tok" -H 'Content-Type: application/json' \
  -d '{"url":"https://quench-works.com/smoke","title":"quench smoke","tag_names":["smoke"]}' >/dev/null
out="$(curl -fsS 'http://127.0.0.1:19090/api/bookmarks/?q=smoke' -H "Authorization: Token $tok")"
grep -q 'quench-works.com/smoke' <<<"$out" || { echo "bookmark not listed: $out"; exit 1; }
echo "API: created and listed a bookmark"
if docker logs "$NAME" 2>&1 | grep -qE 'Traceback|read-only file system'; then echo "errors in the log:"; docker logs "$NAME" 2>&1 | grep -E -A3 'Traceback|read-only' | head -20; exit 1; fi
echo "smoke test passed: superuser from env, /health, REST API create and search, read-only root, user 1001"
