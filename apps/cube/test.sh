#!/usr/bin/env bash
# Smoke test for a built cube image. Usage: test.sh <image-ref> [version]
# Boots the server read-only with a one-cube model and no reachable database, checks /livez,
# and checks the SQL API port: Cube's SQL API (cubesql) lives in the native addon built in
# melange.yaml, so a listening 15432 proves the from-source index.node loads and runs.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="cube-smoke-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "${model:-}"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint /usr/bin/node "$IMAGE" -p 'require("/usr/lib/cube/node_modules/@cubejs-backend/server/package.json").version')"
echo "cube $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }

# the model sits under $PWD: Docker Desktop cannot mount /tmp
model="$(mktemp -d "${PWD}/.cube-model.XXXXXX")"
mkdir -p "$model/model/cubes"
cat > "$model/model/cubes/orders.yml" <<'EOF'
cubes:
  - name: orders
    sql_table: public.orders
    measures:
      - name: count
        type: count
EOF
chmod -R a+rX "$model"

docker run -d --name "$NAME" --read-only --tmpfs /tmp \
  -v "$model/model:/cube/conf/model:ro" \
  -e CUBEJS_DB_TYPE=postgres -e CUBEJS_DB_HOST=127.0.0.1 -e CUBEJS_DB_NAME=x \
  -e CUBEJS_DB_USER=x -e CUBEJS_DB_PASS=x -e CUBEJS_API_SECRET=smoke-test-secret \
  -e CUBEJS_PG_SQL_PORT=15432 -e CUBEJS_SQL_USER=cube -e CUBEJS_SQL_PASSWORD=cube \
  -p 127.0.0.1:14000:4000 -p 127.0.0.1:15432:15432 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 90); do
  curl -fsS http://127.0.0.1:14000/livez >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "/livez never answered"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

sql=0
for _ in $(seq 1 30); do
  (exec 3<>/dev/tcp/127.0.0.1/15432) 2>/dev/null && { sql=1; break; }
  sleep 1
done
[ "$sql" = 1 ] || { echo "SQL API (native addon) not listening on 15432"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
echo "smoke test passed: /livez ok, SQL API listening (native addon), user 1001"
