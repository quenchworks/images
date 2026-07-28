#!/usr/bin/env bash
# Smoke test for a built Varnish Cache image. Usage: test.sh <image-ref>
#
# A TCP-accept probe is worthless here (docker's port proxy accepts before and
# after the process is alive, and varnishd's most likely failure -- "Running
# C-compiler failed" -- happens AFTER the listener would look fine), so this test
# insists on a real HTTP response that only Varnish can produce:
#   1. varnishd -V reports a real version.
#   2. The image boots with a READ-ONLY rootfs, writable tmpfs on the working dir
#      (-n) and /tmp, as nonroot uid 1001 -- exactly how the chart runs it. The
#      working-dir tmpfs MUST be mounted `exec`: varnishd dlopen()s the VCL it just
#      compiled from there, and docker's tmpfs default of noexec makes it refuse to
#      start at all ("cannot reside on a file system mounted noexec"). Kubernetes
#      emptyDir volumes are exec-capable by default, so the chart needs nothing
#      special -- but a cluster policy that forces noexec would break Varnish.
#   3. GET / returns 200 AND carries X-Varnish + Server: Varnish: proof that the
#      VCL compiled, the child started, and the cache itself answered.
#   4. A SECOND request gets its OWN X-Varnish transaction id -- the xid counter
#      moved, so a live child is minting transactions rather than something
#      replaying one canned response.
#   5. The varnishadm CLI on 6082 answers with its 107 auth challenge (a real
#      protocol reply, not just an open socket).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-varnish-smoke-$$"
PORT=8080
CLI_PORT=6082

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# --- 1. the binary runs and reports its version ----------------------------
ver_out="$(docker run --rm --entrypoint /usr/bin/varnishd "$IMAGE" -V 2>&1 | head -1)"
echo "$ver_out"
printf '%s\n' "$ver_out" | grep -qE 'varnish-[0-9]+\.[0-9]+\.[0-9]+' || {
  echo "varnishd -V did not report a version"; exit 1; }

# --- 2. boot it the way the chart does -------------------------------------
echo "starting $IMAGE (read-only rootfs + tmpfs /var/lib/varnish and /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /var/lib/varnish:rw,exec,uid=1001,gid=1001 \
  --tmpfs /tmp:rw,mode=1777 \
  -p "127.0.0.1:${PORT}:8080" \
  -p "127.0.0.1:${CLI_PORT}:6082" \
  "$IMAGE" >/dev/null

hdr=""
for i in $(seq 1 45); do
  hdr="$(curl -sS -m 5 -D - -o /dev/null "http://127.0.0.1:${PORT}/" 2>/dev/null || true)"
  printf '%s' "$hdr" | grep -qi '^HTTP/1.1 200' && break
  docker ps --filter "name=$NAME" --filter "status=running" --format '{{.Names}}' \
    | grep -q "$NAME" || { echo "container died during startup:"; docker logs "$NAME"; exit 1; }
  [ "$i" = 45 ] && { echo "varnish never served a 200"; docker logs "$NAME"; exit 1; }
  sleep 1
done

# --- 3. it is VARNISH that answered ----------------------------------------
printf '%s\n' "$hdr" | sed -n '1,12p'
printf '%s' "$hdr" | grep -qi '^HTTP/1.1 200'   || { echo "no 200 from :$PORT"; exit 1; }
printf '%s' "$hdr" | grep -qi '^X-Varnish:'     || { echo "no X-Varnish header -- something other than Varnish answered"; exit 1; }
printf '%s' "$hdr" | grep -qi '^Server: *Varnish' || { echo "no 'Server: Varnish' header"; exit 1; }
xid="$(printf '%s' "$hdr" | grep -i '^X-Varnish:' | tr -dc '0-9')"
[ -n "$xid" ] || { echo "X-Varnish carried no transaction id"; exit 1; }
echo "GET / -> 200 with X-Varnish: $xid"

body="$(curl -fsS -m 5 "http://127.0.0.1:${PORT}/")"
printf '%s' "$body" | grep -q 'quench-varnish' || { echo "unexpected body: $body"; exit 1; }

# --- 4. a second, independent transaction through the same cache -----------
hdr2="$(curl -sS -m 5 -D - -o /dev/null "http://127.0.0.1:${PORT}/second" 2>/dev/null)"
xid2="$(printf '%s' "$hdr2" | grep -i '^X-Varnish:' | tr -dc '0-9')"
[ -n "$xid2" ] && [ "$xid2" != "$xid" ] \
  || { echo "second request did not get its own X-Varnish xid (got '$xid2', first was '$xid')"; exit 1; }
# No Age assertion on purpose: Varnish emits Age only for objects it delivers from
# storage, and the default VCL answers from vcl_synth (no backend), so Age is
# legitimately absent here. The chart's gate is what exercises a real cached fetch.
echo "second request served with its own xid: $xid2"

# --- 5. varnishadm CLI on 6082 --------------------------------------------
# The CLI greets a new connection with "107 <n>" (auth challenge) because -S left
# the secret in the working dir. Reading that banner proves the manager process is
# serving the admin protocol, not merely that the port is bound.
banner="$( (exec 3<>/dev/tcp/127.0.0.1/$CLI_PORT; timeout 10 head -c 3 <&3) 2>/dev/null || true)"
[ "$banner" = "107" ] || { echo "varnishadm CLI did not send the 107 auth challenge (got '$banner')"; docker logs "$NAME"; exit 1; }
echo "varnishadm CLI answered on :$CLI_PORT (auth challenge $banner)"

# no read-only-rootfs damage
if docker logs "$NAME" 2>&1 | grep -qiE 'read-only file system|permission denied|C-compiler failed'; then
  echo "found read-only / permission / compiler errors in logs:"; docker logs "$NAME"; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user, read-only rootfs, cache served HTTP)"
