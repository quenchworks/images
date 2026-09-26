#!/usr/bin/env bash
# Smoke test for a built RethinkDB image. Usage: test.sh <image-ref> [version]
# Starts the server and uses the official Python driver from the runner to
# create a database and table, insert a document and read it back, and checks
# the reported version and the web admin UI.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-rethinkdb-smoke-$$"
VENV="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$VENV"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" -p 127.0.0.1:28015:28015 -p 127.0.0.1:18080:8080 "$IMAGE" >/dev/null
for i in $(seq 1 60); do
  docker logs "$NAME" 2>&1 | grep -q "Server ready" && break
  [ "$i" = 60 ] && { echo "server never ready"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
  sleep 1
done
code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/)"
echo "web admin: HTTP $code"
[ "$code" = 200 ] || { echo "web admin not served"; exit 1; }

python3 -m venv "$VENV" && "$VENV/bin/pip" install -q rethinkdb==2.4.10.post1
"$VENV/bin/python" - "$WANT" <<'PY'
import sys
from rethinkdb import r
c = r.connect(host="127.0.0.1", port=28015)
r.db_create("smoke").run(c)
r.db("smoke").table_create("t").run(c)
r.db("smoke").table("t").insert({"id": 1, "name": "quench"}).run(c)
doc = r.db("smoke").table("t").get(1).run(c)
assert doc == {"id": 1, "name": "quench"}, doc
ver = r.db("rethinkdb").table("server_status").nth(0)["process"]["version"].run(c)
print("document:", doc)
print("version:", ver)
assert not sys.argv[1] or ver.startswith("rethinkdb " + sys.argv[1]), ver
PY

echo "smoke test passed (rethinkdb ${WANT:-?}: create, insert, read over the driver; web admin; nonroot user: $user)"
