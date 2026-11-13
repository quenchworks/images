#!/usr/bin/env bash
# Smoke test for a built ClamAV image. Usage: test.sh <image-ref> [version]
#
# clamd will not start without a signature database, and downloading the real
# one from CI is slow and rate-limited. So the test writes a one-line database
# of its own: the MD5 of the EICAR test file (.hdb format), then:
#   1. clamd --version, freshclam --version and clamscan --version run.
#   2. clamd boots as uid 1001 on a READ-ONLY rootfs, database mounted
#      read-only, /tmp writable: the way the chart runs it.
#   3. The clamd TCP protocol answers PING with PONG.
#   4. INSTREAM of the EICAR string is reported FOUND with our signature name,
#      and a clean payload is reported OK: the engine loaded the database and
#      scanned both.
#   5. clamscan, run as a one-shot container, flags the same file (exit 1).
set -euo pipefail
# qw (tools/qw, Go) provides the helpers this test needs; go run reuses the build cache.
QWD="$(cd "$(dirname "$0")/../../tools/qw" && pwd)"; qw() { go -C "$QWD" run . "$@"; }

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
NAME="quench-clamav-smoke-$$"
PORT=13310
# Under $HOME, not /tmp: Docker Desktop cannot bind-mount /tmp paths.
work="$(mktemp -d "$HOME/.quench-clamav-test.XXXXXX")"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT

for bin in clamd freshclam clamscan; do
  out="$(docker run --rm --entrypoint "/usr/bin/$bin" "$IMAGE" --version 2>&1 || true)"
  echo "$bin: $out"
  grep -qE '^ClamAV [0-9]+\.[0-9]+\.[0-9]+' <<<"$out" || { echo "$bin reported no version"; exit 1; }
  if [ -n "${2:-}" ]; then grep -qF "ClamAV ${2}" <<<"$out" || { echo "expected $2"; exit 1; }; fi
done

# EICAR, assembled so this file itself is not a match for a scanner.
eicar='X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR'
eicar="${eicar}-STANDARD-ANTIVIRUS-TEST-FILE!\$H+H*"
printf '%s' "$eicar" > "$work/eicar.txt"
md5="$(md5sum "$work/eicar.txt" | cut -d' ' -f1)"
size="$(wc -c < "$work/eicar.txt" | tr -d ' ')"
mkdir -p "$work/db"
printf '%s:%s:Quench.Test.EICAR\n' "$md5" "$size" > "$work/db/quench.hdb"
chmod -R a+rX "$work"

docker run -d --name "$NAME" --read-only --tmpfs /tmp:rw,mode=1777 \
  -v "$work/db:/var/lib/clamav:ro" -p "127.0.0.1:${PORT}:3310" "$IMAGE" >/dev/null

# clamd speaks z-terminated commands; INSTREAM sends <len:4 BE><bytes>, then a zero length
clamd_cmd() {
  if [ "$1" = PING ]; then
    qw tlsprobe --plain --addr "127.0.0.1:$PORT" --send 'zPING\x00' --print
  else
    qw tlsprobe --plain --addr "127.0.0.1:$PORT" --print \
      --send-hex "$(printf '7a494e53545245414d00%08x' "$(wc -c <"$2")"; od -An -tx1 -v "$2" | tr -d ' \n'; printf 00000000)"
  fi
}

pong=""
for i in $(seq 1 60); do
  pong="$(clamd_cmd PING 2>/dev/null || true)"
  [ "$pong" = PONG ] && break
  docker ps --filter "name=$NAME" --filter status=running --format '{{.Names}}' | grep -q "$NAME" \
    || { echo "clamd died during startup:"; docker logs "$NAME"; exit 1; }
  [ "$i" = 60 ] && { echo "clamd never answered PING"; docker logs "$NAME"; exit 1; }
  sleep 1
done
echo "PING -> $pong"

hit="$(clamd_cmd INSTREAM "$work/eicar.txt")"
echo "EICAR -> $hit"
grep -q 'Quench.Test.EICAR.* FOUND' <<<"$hit" || { echo "EICAR was not detected"; docker logs "$NAME"; exit 1; }
printf 'nothing to see here\n' > "$work/clean.txt"
ok="$(clamd_cmd INSTREAM "$work/clean.txt")"
echo "clean -> $ok"
[ "$ok" = "stream: OK" ] || { echo "clean payload not OK"; exit 1; }

set +e
docker run --rm --read-only --tmpfs /tmp -v "$work/db:/var/lib/clamav:ro" -v "$work:/scan:ro" \
  --entrypoint /usr/bin/clamscan "$IMAGE" --no-summary /scan/eicar.txt
rc=$?
set -e
[ "$rc" = 1 ] || { echo "clamscan exit $rc, expected 1 (virus found)"; exit 1; }
echo "clamscan flagged the file (exit 1)"

if docker logs "$NAME" 2>&1 | grep -qE 'Read-only file system|[Pp]ermission denied|ERROR'; then
  echo "found errors in logs:"; docker logs "$NAME"; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot $user, read-only rootfs, clamd detected EICAR)"
