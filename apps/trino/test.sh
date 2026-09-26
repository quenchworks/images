#!/usr/bin/env bash
# Smoke test for a built Trino image. Usage: test.sh <image-ref> [version]
# Boots the default single-node coordinator, waits for /v1/info to report
# starting=false, then runs real queries over the client REST protocol: a tpch
# aggregate, a join, and a CREATE/SELECT round trip in the memory catalog.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-trino-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

docker run -d --name "$NAME" -p 127.0.0.1:8080:8080 "$IMAGE" >/dev/null
for i in $(seq 1 90); do
  info="$(curl -fsS http://127.0.0.1:8080/v1/info 2>/dev/null || true)"
  grep -q '"starting":false' <<<"$info" && break
  [ "$i" = 90 ] && { echo "trino did not finish starting"; docker logs "$NAME" | tail -50; exit 1; }
  sleep 2
done
echo "info: $info"
if [ -n "$WANT" ]; then
  grep -q "\"version\":\"$WANT\"" <<<"$info" || { echo "expected version $WANT"; exit 1; }
fi

# Runs one statement to completion and prints every data page as JSON.
query() {
  local r n
  r="$(curl -fsS -X POST -H 'X-Trino-User: smoke' --data "$1" http://127.0.0.1:8080/v1/statement)"
  while :; do
    python3 - "$r" <<'PY'
import json, sys
j = json.loads(sys.argv[1])
if j.get("error"):
    sys.exit("query failed: " + j["error"].get("message", ""))
if j.get("data"):
    print(json.dumps(j["data"]))
PY
    n="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("nextUri", ""))' "$r")"
    [ -z "$n" ] && return 0
    r="$(curl -fsS -H 'X-Trino-User: smoke' "$n")"
  done
}

out="$(query 'SELECT count(*) FROM tpch.tiny.nation')"; echo "nation: $out"
grep -qx '\[\[25\]\]' <<<"$out" || { echo "tpch count wrong"; exit 1; }
out="$(query 'SELECT count(*) FROM tpch.tiny.customer c JOIN tpch.tiny.nation n ON c.nationkey = n.nationkey')"
grep -qx '\[\[1500\]\]' <<<"$out" || { echo "tpch join wrong: $out"; exit 1; }
query 'CREATE TABLE memory.default.smoke AS SELECT 42 AS x' >/dev/null
out="$(query 'SELECT x FROM memory.default.smoke')"
grep -qx '\[\[42\]\]' <<<"$out" || { echo "memory round trip wrong: $out"; exit 1; }

echo "smoke test passed (trino ${WANT:-?}, nonroot user: $user)"
