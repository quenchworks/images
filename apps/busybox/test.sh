#!/usr/bin/env bash
# Smoke test for a built BusyBox toolbox image. Usage: test.sh <image-ref>
# Unlike a service image, this one ships a shell by design, so we exercise applets
# directly under a READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

echo "== banner + core applets (read-only rootfs) =="
# Override the entrypoint so the CMD below is parsed as `sh -c <script>` directly,
# rather than appended after the image's default `/bin/sh` entrypoint (which would
# make busybox treat the literal token "sh" as a script filename to open).
docker run --rm --read-only --entrypoint /bin/sh "$IMAGE" -c '
  set -e
  echo ok
  busybox | head -1
  ls / >/dev/null
  id
  echo "hello" | sed "s/hello/sed-ok/"
  echo "grepline" | grep grep >/dev/null && echo grep-ok
  echo "a b c" | awk "{print \$2}" | grep -q b && echo awk-ok
  cat /etc/os-release >/dev/null 2>&1 || true
  wget --help 2>&1 | head -1 >/dev/null && echo wget-ok
  for a in sh ls cat cp mv rm mkdir ln sed grep awk wget; do
    [ -L "/bin/$a" ] || { echo "missing applet symlink: $a"; exit 1; }
  done
  echo "applet symlinks ok"
'

echo "== nonroot uid 1001 =="
uid="$(docker run --rm --read-only --entrypoint /bin/id "$IMAGE" -u)"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot uid: $uid)"
