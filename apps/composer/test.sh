#!/usr/bin/env bash
# Smoke test for a built quench-composer image. Usage: test.sh <image-ref> <phpminor>
# where <phpminor> is the expected underlying PHP minor, e.g. 8.4.
#
# This is a BUILD/PACKAGE-TOOL image: `composer` (the phar run through the php
# interpreter) IS the entrypoint, there is no long-running service. We exercise
# composer + php + git directly under a READ-ONLY rootfs and confirm the nonroot
# uid. COMPOSER_HOME/HOME live on the writable /tmp tmpfs.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <phpminor>}"
PHPMINOR="${2:?usage: test.sh <image-ref> <phpminor>}"   # e.g. 8.4

echo "== composer --version reports a 2.x release and the bundled PHP (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>&1)"
echo "$ver"
case "$ver" in
  *"Composer version 2."*) : ;;
  *) echo "expected 'Composer version 2.*', got '$ver'"; exit 1 ;;
esac
case "$ver" in
  *"PHP version $PHPMINOR."*) : ;;
  *) echo "expected composer to report 'PHP version $PHPMINOR.*', got '$ver'"; exit 1 ;;
esac

echo "== php -v matches $PHPMINOR =="
pver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/php "$IMAGE" -v 2>&1)"
echo "$pver" | head -1
case "$pver" in
  "PHP $PHPMINOR."*) : ;;
  *) echo "expected 'PHP $PHPMINOR.*', got '$pver'"; exit 1 ;;
esac

echo "== git is present (composer needs it for VCS installs) =="
gver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/git "$IMAGE" --version 2>&1)"
echo "$gver"
case "$gver" in
  "git version "*) : ;;
  *) echo "expected git version output, got '$gver'"; exit 1 ;;
esac

echo "== composer diagnose runs (exercises the writable COMPOSER_HOME on /tmp) =="
docker run --rm --read-only --tmpfs /tmp "$IMAGE" config --global --list >/dev/null 2>&1 \
  || { echo "composer could not read/write its global config under /tmp"; exit 1; }

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/php "$IMAGE" -r 'echo getmyuid();')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
if ! docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/php "$IMAGE" \
     -r 'exit(@file_put_contents("/should-fail","x") === false ? 0 : 1);' >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (composer ${ver#Composer version }, PHP $PHPMINOR, nonroot uid: $uid, read-only rootfs)"
