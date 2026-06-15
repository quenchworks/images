#!/usr/bin/env bash
# Smoke test for a built quench-perl image. Usage: test.sh <image-ref> <major>
# where <major> is the expected Perl major line, e.g. 5.
#
# This is a LANGUAGE/BASE image: `perl` IS the entrypoint, there is no
# long-running service to ping. We exercise perl (-v, -e, run a .pl script)
# under a READ-ONLY rootfs and confirm the nonroot uid.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <major>}"
MAJOR="${2:?usage: test.sh <image-ref> <major>}"   # e.g. 5

echo "== perl -v reports perl $MAJOR (default entrypoint) =="
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -v 2>&1 | grep -i 'this is perl')"
echo "$ver"
case "$ver" in
  *"perl $MAJOR,"*) : ;;
  *) echo "expected 'perl $MAJOR,', got '$ver'"; exit 1 ;;
esac

echo "== perl -e runs =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -e 'print "quench-perl"')"
[ "$out" = "quench-perl" ] || { echo "-e output wrong: '$out'"; exit 1; }

echo "== core modules load (strict, warnings, Data::Dumper) =="
docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
  -e 'use strict; use warnings; use Data::Dumper; print "mods-ok\n"' | grep -q 'mods-ok'

echo "== runs as nonroot uid 1001 =="
uid="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -e 'print $<')"
[ "$uid" = "1001" ] || { echo "expected uid 1001, got '$uid'"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== a real .pl script runs (read-only rootfs, project mount RO) =="
# Shell-less image: prepare the script on the HOST and bind-mount it read-only,
# created UNDER the checkout for portable mounts (CI + Docker Desktop).
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
cat > "$WORK/h.pl" <<'EOF'
use strict; use warnings;
print "quench-perl-script\n";
EOF
out2="$(docker run --rm --read-only --tmpfs /tmp -v "$WORK:/work:ro" -w /work "$IMAGE" h.pl)"
[ "$out2" = "quench-perl-script" ] || { echo ".pl run output wrong: '$out2'"; exit 1; }

echo "== read-only rootfs is enforced (write to / must fail) =="
if docker run --rm --read-only --tmpfs /tmp "$IMAGE" \
     -e 'open(my $f, ">", "/should-fail") or exit 1; exit 0' >/dev/null 2>&1; then
  echo "rootfs was writable, expected read-only"; exit 1
fi

echo "smoke test passed (Perl $MAJOR, nonroot uid: $uid, read-only rootfs)"
