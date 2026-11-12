#!/usr/bin/env bash
# Smoke test for a built Glance image. Usage: test.sh <image-ref> [version]
# Glance runs on a read-only root with a mounted config: one page with an HTML widget and a monitor
# widget that probes Glance's own /api/healthz. The rendered page content must carry the HTML
# marker and the monitored site reported up.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="glance-smoke-$$"
VOL="$NAME-config"
URL=http://127.0.0.1:18080
cleanup() { docker rm -f "$NAME" "$NAME-cfg" >/dev/null 2>&1 || true; docker volume rm "$VOL" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm --entrypoint /usr/bin/glance "$IMAGE" --version 2>&1)"
echo "glance $v"
[ -z "$WANT" ] || grep -qF "$WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

# a volume, not a bind mount: the scratchpad is not shared with the Docker VM
docker volume create "$VOL" >/dev/null
docker create --name "$NAME-cfg" -v "$VOL:/cfg" "$IMAGE" >/dev/null
cfg="$(mktemp "$PWD/.glance-smoke.XXXXXX")"; trap 'rm -f "$cfg"; cleanup' EXIT
cat > "$cfg" <<'EOF'
server:
  port: 8080
pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets:
          - type: html
            source: <p id="quench-marker">quench-glance-smoke</p>
          - type: monitor
            title: Services
            sites:
              - title: Glance itself
                url: http://127.0.0.1:8080/api/healthz
EOF
chmod 0644 "$cfg"
docker cp "$cfg" "$NAME-cfg:/cfg/glance.yml"
docker run -d --name "$NAME" --read-only --tmpfs /tmp:uid=1001,gid=1001 -v "$VOL:/app/config:ro" \
  -p 127.0.0.1:18080:8080 "$IMAGE" >/dev/null
for i in $(seq 1 30); do curl -fsS "$URL/api/healthz" >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { echo "/api/healthz never answered"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }; sleep 1; done
curl -fsS "$URL/" | grep -q '<html' || { echo "no page HTML at /"; exit 1; }
content=""
for _ in $(seq 1 20); do
  content="$(curl -fsS "$URL/api/pages/home/content/" 2>/dev/null || true)"
  grep -q 'quench-glance-smoke' <<<"$content" && grep -q 'Glance itself' <<<"$content" && break; sleep 1
done
grep -q 'quench-glance-smoke' <<<"$content" || { echo "HTML widget not rendered"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }
grep -q 'Glance itself' <<<"$content" || { echo "monitor widget not rendered"; exit 1; }
# the monitor template draws the status icon with --color-positive when the site is up
grep -q 'fill="var(--color-positive)"' <<<"$content" || { echo "monitored site not reported up"; exit 1; }
if grep -q 'fill="var(--color-negative)"' <<<"$content"; then echo "monitored site reported down"; exit 1; fi
echo "  page content: HTML widget and monitor (site up) rendered"
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|panic'; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed (glance ${WANT:-?}: config from a volume, HTML + monitor widgets rendered, read-only root; user $user)"
