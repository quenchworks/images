#!/usr/bin/env bash
# Smoke test for a built quench-elixir image. Usage: test.sh <image-ref> <minor>
# where <minor> is the expected Elixir minor line, e.g. 1.18.
#
# This is a LANGUAGE/BASE image: `elixir` IS the entrypoint, there is no
# long-running service to ping. We exercise elixir (version, -e script, a real
# .exs file) under a READ-ONLY rootfs and confirm the nonroot uid. Writable
# HOME/MIX_HOME/HEX_HOME are baked to /tmp in the image env.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <minor>}"
MINOR="${2:?usage: test.sh <image-ref> <minor>}"   # e.g. 1.18

echo "== elixir --version reports $MINOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>&1)"
echo "$ver"
echo "$ver" | grep -q "Elixir $MINOR." || { echo "expected 'Elixir $MINOR.', got '$ver'"; exit 1; }

echo "== no latin1 encoding warning (UTF-8 forced via +fnu) =="
if echo "$ver" | grep -qi 'native name encoding of latin1'; then
  echo "latin1 warning present -- UTF-8 not configured"; exit 1
fi

echo "== elixir -e runs a script =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -e 'IO.puts("quench-elixir " <> Integer.to_string(1 + 1))' 2>&1)"
[ "$out" = "quench-elixir 2" ] || { echo "-e output wrong: '$out'"; exit 1; }

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -e 'IO.puts(:os.cmd(~c"id -u") |> List.to_string() |> String.trim())' 2>&1)"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== a real .exs script runs (read-only rootfs, project mount RO) =="
# BEAM needs the busybox /bin/sh; we still prepare the script on the HOST and
# bind-mount it read-only (portable for CI + Docker Desktop).
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
cat > "$WORK/h.exs" <<'EOF'
IO.puts("quench-elixir-script")
EOF
out2="$(docker run --rm --read-only --tmpfs /tmp -v "$WORK:/work:ro" -w /work "$IMAGE" /work/h.exs 2>&1)"
[ "$out2" = "quench-elixir-script" ] || { echo ".exs output wrong: '$out2'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
if docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
     -e 'case File.write("/should-fail", "x") do :ok -> System.halt(0); _ -> System.halt(1) end' \
     >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (Elixir $MINOR, nonroot uid: $uid, read-only rootfs)"
