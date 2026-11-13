#!/usr/bin/env bash
# Smoke test for a built sish image. Usage: test.sh <image-ref> [version]
# A local HTTP server (python on the host) is exposed through sish with a plain `ssh -R`, then
# fetched through sish's HTTP port by its subdomain; an unknown subdomain must not reach it.
# sish keeps a socket file per tunnel in /tmp, so /tmp is writable as it is in a pod.
set -euo pipefail
# qw (tools/qw, Go) provides the helpers this test needs; go run reuses the build cache.
QWD="$(cd "$(dirname "$0")/../../tools/qw" && pwd)"; qw() { go -C "$QWD" run . "$@"; }
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="sish-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
pids=()
cleanup() { for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done; docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$v" | head -1
[ -z "$WANT" ] || grep -qF "v$WANT" <<<"$v" || { echo "expected v$WANT"; exit 1; }

docker run -d --name "$NAME" --read-only --tmpfs /tmp:uid=1001,gid=1001 --tmpfs /private:uid=1001,gid=1001 --tmpfs /pubkeys:uid=1001,gid=1001 -p 127.0.0.1:12222:2222 -p 127.0.0.1:18080:8080 "$IMAGE" \
  --ssh-address=:2222 --http-address=:8080 --domain=sish.test --authentication=false \
  --bind-random-subdomains=false --private-keys-directory=/private \
  --authentication-keys-directory=/pubkeys >/dev/null

echo quench-tunneled > "$WORK/index.html"
go -C "$QWD" build -o "$WORK/qw" . && { "$WORK/qw" serve --dir "$WORK" --addr 127.0.0.1:18888 >/dev/null 2>&1 & pids+=($!); }
ssh-keygen -q -t ed25519 -N '' -f "$WORK/id" >/dev/null
# wait for sish itself: Docker's port proxy accepts TCP before the container listens
for _ in $(seq 1 30); do grep -q 'starting SSH service' <<<"$(docker logs "$NAME" 2>&1)" && break; sleep 1; done
sleep 1
ssh -i "$WORK/id" -p 12222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ExitOnForwardFailure=yes \
  -N -R app:80:127.0.0.1:18888 127.0.0.1 >"$WORK/ssh.log" 2>&1 & pids+=($!)
out=""
for _ in $(seq 1 30); do out="$(curl -sS --max-time 5 -H 'Host: app.sish.test' http://127.0.0.1:18080/ 2>/dev/null || true)"; [ "$out" = quench-tunneled ] && break; sleep 1; done
[ "$out" = quench-tunneled ] || { echo "tunnel not reached: $out"; echo "ssh:"; cat "$WORK/ssh.log"; docker logs "$NAME" 2>&1 | grep -v '"http request"' | tail -15; exit 1; }
echo "through the ssh -R tunnel: $out"
other="$(curl -sS --max-time 5 -H 'Host: other.sish.test' http://127.0.0.1:18080/ || true)"
[ "$other" != quench-tunneled ] || { echo "an unknown subdomain reached the tunnel"; exit 1; }
if docker logs "$NAME" 2>&1 | grep -qE 'panic|read-only file system'; then echo "crashed or wrote to the root"; exit 1; fi
echo "smoke test passed: HTTP tunnel over plain ssh -R, unknown subdomain refused, user 1001"
