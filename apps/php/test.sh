#!/usr/bin/env bash
# Smoke test for a built quench-php image. Usage: test.sh <image-ref> <version>
# where <version> is the expected PHP version (full X.Y.Z, e.g. 8.4.22; a bare
# minor like 8.4 also works as a prefix).
#
# This is a LANGUAGE/BASE image: the interpreter IS the entrypoint, there is no
# long-running service to ping. So we exercise php + its extensions directly
# under a READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <version>}"
EXPECT="${2:?usage: test.sh <image-ref> <version>}"   # e.g. 8.4.22

echo "== php -v matches $EXPECT (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -v 2>&1)"
echo "$ver"
# php -v prints "PHP X.Y.Z (cli) ...": match EXPECT at a word boundary so a full
# X.Y.Z (followed by a space) and a bare minor (followed by a dot) both pass.
case "$ver" in
  "PHP $EXPECT "* | "PHP $EXPECT."*) : ;;
  *) echo "expected 'PHP $EXPECT', got '$ver'"; exit 1 ;;
esac

echo "== php -m lists the expected extensions =="
mods="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -m 2>&1)"
echo "$mods"
for ext in openssl mbstring curl json ctype iconv fileinfo sodium bcmath sockets intl gd zip Phar dom xml xmlreader xmlwriter SimpleXML PDO pdo_sqlite pdo_mysql pdo_pgsql; do
  echo "$mods" | grep -qiE "^${ext}$" || { echo "missing extension: $ext"; exit 1; }
done

echo "== a real script runs and core/extension calls work =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -r 'echo "quench-" . "php" . "|" . hash("sha256","x") . "|" . json_encode(["ok"=>true]);')"
case "$out" in
  "quench-php|2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881|"*'{"ok":true}') : ;;
  *) echo "script output wrong: '$out'"; exit 1 ;;
esac

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -r 'echo getmyuid();')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
# PHP returns false (not a non-zero exit) when a write fails, so assert inside
# the script: exit 1 only if the write unexpectedly SUCCEEDED.
if ! docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
     -r 'exit(@file_put_contents("/should-fail","x") === false ? 0 : 1);' >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (PHP $MINOR, nonroot uid: $uid, read-only rootfs)"
