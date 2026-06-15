#!/usr/bin/env bash
# Smoke test for a built quench-erlang image. Usage: test.sh <image-ref> <major>
# where <major> is the expected OTP major, e.g. 28.
#
# This is a LANGUAGE/BASE image: `erl` IS the entrypoint, there is no
# long-running service to ping. We exercise the BEAM (otp_release, an -eval
# expression, escript) under a READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <major>}"
MAJOR="${2:?usage: test.sh <image-ref> <major>}"   # e.g. 28

echo "== erl reports OTP release $MAJOR (default entrypoint) =="
otp="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -noshell -eval 'io:format("~s", [erlang:system_info(otp_release)]), halt().' 2>&1)"
echo "otp_release=$otp"
[ "$otp" = "$MAJOR" ] || { echo "expected OTP release $MAJOR, got '$otp'"; exit 1; }

echo "== erl -eval evaluates an expression =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -noshell -eval 'io:format("eval-~p", [1 + 1]), halt().' 2>&1)"
[ "$out" = "eval-2" ] || { echo "eval output wrong: '$out'"; exit 1; }

echo "== runs as nonroot uid 1001 (os:cmd shells out via busybox) =="
uid="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -noshell -eval 'io:format("~s", [string:trim(os:cmd("id -u"))]), halt().' 2>&1)"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== escript runs a real script (read-only rootfs, project mount RO) =="
# Shell-less except for the busybox /bin/sh the BEAM needs; we still prepare the
# escript on the HOST and bind-mount it read-only (portable for CI + Desktop).
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
cat > "$WORK/h.escript" <<'EOF'
#!/usr/bin/env escript
main(_) -> io:format("quench-erlang~n").
EOF
out2="$(docker run --rm --read-only --tmpfs /tmp -v "$WORK:/work:ro" -w /work \
  --entrypoint /usr/bin/escript "$IMAGE" h.escript 2>&1)"
[ "$out2" = "quench-erlang" ] || { echo "escript output wrong: '$out2'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
if docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
     -noshell -eval 'case file:write_file("/should-fail", <<"x">>) of ok -> halt(0); _ -> halt(1) end.' \
     >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (Erlang/OTP $MAJOR, nonroot uid: $uid, read-only rootfs)"
