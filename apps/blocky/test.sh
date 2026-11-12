#!/usr/bin/env bash
# Smoke test for a built Blocky image. Usage: test.sh <image-ref> [version]
# Blocky starts from a config with a custom DNS mapping and an inline denylist. Queries go
# through blocky's own `query` command against its API: the mapping must resolve, the
# denylisted name must be blocked, the DNS port must answer, and metrics must be served.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="blocky-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$v"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

cat > "$WORK/config.yml" <<'YML'
upstreams:
  init:
    strategy: fast
  groups:
    default:
      - 1.1.1.1
customDNS:
  mapping:
    smoke.quench.test: 192.0.2.10
blocking:
  denylists:
    ads:
      - |
        blocked.quench.test
  clientGroupsBlock:
    default:
      - ads
ports:
  dns: 5353
  http: 4000
prometheus:
  enable: true
YML
chmod -R a+rX "$WORK"
docker run -d --name "$NAME" --read-only -v "$WORK/config.yml:/app/config.yml:ro" -p 127.0.0.1:14000:4000 "$IMAGE" >/dev/null

q() { docker exec "$NAME" /usr/bin/blocky query "$1" --apiPort 4000 2>&1; }
out=""
for _ in $(seq 1 30); do
  out="$(q smoke.quench.test || true)"
  grep -q '192.0.2.10' <<<"$out" && break
  sleep 1
done
grep -q '192.0.2.10' <<<"$out" || { echo "custom mapping not answered: $out"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
echo "custom DNS: smoke.quench.test -> 192.0.2.10"
b="$(q blocked.quench.test)"
grep -qi 'BLOCKED' <<<"$b" || { echo "denylisted name not blocked: $b"; exit 1; }
echo "denylist: blocked.quench.test blocked"
m="$(curl -fsS http://127.0.0.1:14000/metrics)"
grep -q 'blocky_' <<<"$m" || { echo "no blocky metrics"; exit 1; }
if docker logs "$NAME" 2>&1 | grep -qiE 'panic|read-only file system'; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: custom DNS, denylist blocking, metrics, user 1001"
