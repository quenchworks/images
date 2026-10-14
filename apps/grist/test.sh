#!/usr/bin/env bash
# Smoke test for a built grist image. Usage: test.sh <image-ref> [version]
# Runs it on a read-only root with /persist as the only writable volume, no capabilities and
# no-new-privileges (what the chart's pod gets), creates a document through the REST API
# (authenticated with GRIST_BOOT_KEY as the install admin), adds a table with formula columns,
# writes a row and reads back what the gVisor sandbox computed. Column H returns the hostname
# the engine sees: run.py names the runsc sandbox "gristland", so it proves the formula ran
# inside runsc and not in an unsandboxed python.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="grist-smoke-$$"
KEY="quench-smoke-boot-key-0123456789"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; docker volume rm "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" -p 'require("/grist/package.json").version')"
echo "grist $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }

docker volume create "$NAME" >/dev/null
docker run -d --name "$NAME" --read-only --tmpfs /tmp:rw,exec -v "$NAME:/persist" \
  --cap-drop ALL --security-opt no-new-privileges \
  -p 127.0.0.1:18484:8484 -e GRIST_BOOT_KEY="$KEY" \
  -e GRIST_DEFAULT_EMAIL=admin@example.com "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 90); do
  curl -fsS http://127.0.0.1:18484/status >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "/status never came up"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
echo "status ok"

B=http://127.0.0.1:18484/o/docs/api
api() { curl -fsS -m 120 -H "x-boot-key: $KEY" -H 'Content-Type: application/json' "$@"; }
fail() { echo "$1"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }
ws="$(api "$B/orgs/current/workspaces")" || fail "workspaces request failed"
ws="$(jq -r '.[0].id' <<<"$ws")"
[ -n "$ws" ] || fail "no workspace id"
doc="$(api -X POST "$B/workspaces/$ws/docs" -d '{"name":"quench-smoke"}' | tr -d '"')" || fail "doc create failed"
echo "document $doc"
api -X POST "$B/docs/$doc/tables" -d '{"tables":[{"id":"Smoke","columns":[{"id":"A","fields":{"type":"Numeric"}},{"id":"B","fields":{"type":"Numeric","isFormula":true,"formula":"$A * 6 + len(\"abc\")"}},{"id":"H","fields":{"type":"Text","isFormula":true,"formula":"__import__(\"os\").uname()[1]"}}]}]}' >/dev/null || fail "table create failed"
api -X POST "$B/docs/$doc/tables/Smoke/records" -d '{"records":[{"fields":{"A":7}}]}' >/dev/null || fail "record write failed"
out="$(api "$B/docs/$doc/tables/Smoke/records")" || fail "record read failed"
echo "$out"
grep -q '"B":45' <<<"$out" || fail "formula column did not compute 45"
grep -q '"H":"gristland"' <<<"$out" || fail "the formula did not run inside the runsc sandbox"
logs="$(docker logs "$NAME" 2>&1)"
grep -q 'flavor=gvisor' <<<"$logs" || fail "the sandbox flavor was not gvisor"
echo "gVisor sandbox computed the formula"
echo "smoke test passed"
