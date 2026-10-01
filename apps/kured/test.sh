#!/usr/bin/env bash
# Smoke test for a built kured image. Usage: test.sh <image-ref> [expected-version]
#
# kured uses the in-cluster config only, so the test gives it one that points at
# 127.0.0.1:1, where nothing answers: a service-account token tree under /run
# (/var/run is a symlink, so docker cp needs the real path) and the two
# KUBERNETES_SERVICE_* variables. A reboot sentinel is in place before start (kured
# re-reads it only once a minute), so its first /metrics reading must say a reboot is
# required. The chart's kind gate covers the API side (lock annotation on its DaemonSet).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
WANT="${2:-}"
NAME="quench-kured-smoke-$$"
PORT=18081
SA="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$SA"; }
trap cleanup EXIT
fail() { echo "FAIL: $*"; docker logs "$NAME" 2>&1 | tail -20; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

d="$SA/run/secrets/kubernetes.io/serviceaccount"
mkdir -p "$d"
echo smoke > "$d/token"; echo default > "$d/namespace"
openssl req -x509 -newkey rsa:2048 -nodes -keyout /dev/null -subj /CN=smoke -days 1 -out "$d/ca.crt" 2>/dev/null
mkdir -p "$SA/tmp"; touch "$SA/tmp/reboot-required"
chmod -R a+rX "$SA"

docker create --name "$NAME" -p "127.0.0.1:$PORT:8080" -e KURED_NODE_ID=smoke \
  -e KUBERNETES_SERVICE_HOST=127.0.0.1 -e KUBERNETES_SERVICE_PORT=1 \
  "$IMAGE" --period=2s --reboot-sentinel=/tmp/reboot-required >/dev/null
docker cp "$SA/." "$NAME:/"
docker start "$NAME" >/dev/null

metric() { local b; b="$(curl -fsS --max-time 2 "http://127.0.0.1:$PORT/metrics" 2>/dev/null || true)"; grep '^kured_reboot_required{node="smoke"}' <<<"$b" || true; }
for i in $(seq 1 30); do
  m="$(metric)"; [ -n "$m" ] && break
  [ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || fail "kured exited"
  sleep 1
done
[ "$m" = 'kured_reboot_required{node="smoke"} 1' ] || fail "sentinel not detected, metric '$m'"

logs="$(docker logs "$NAME" 2>&1)"
grep -q "Kubernetes Reboot Daemon: ${WANT:-[0-9]}" <<<"$logs" || fail "version line missing or not ${WANT}"
[ "$(docker inspect -f '{{.State.Status}}' "$NAME")" = running ] || fail "kured is not running any more"
docker run --rm --entrypoint /usr/bin/nsenter "$IMAGE" --version >/dev/null || fail "nsenter missing"
echo "smoke test passed (uid $user, sentinel detected, nsenter present)"
