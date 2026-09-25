#!/usr/bin/env bash
# Smoke test for a built gitleaks image. Usage: test.sh <image-ref> [version]
# Real scanning: a directory with a planted AWS key must be reported (exit 1) and a
# clean one must pass (exit 0); `gitleaks git` must find a key committed into history.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" version 2>&1)"
echo "gitleaks $ver"
[ -z "$WANT" ] || echo "$ver" | grep -q "$WANT" || { echo "expected version $WANT"; exit 1; }

WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/leaky" "$WORK/clean"
# the documented AWS example key id, split so this file does not trip scanners itself
printf 'aws_access_key_id = %s%s\naws_secret_access_key = %s%s\n' \
  'AKIA' 'QYLPMN5HHHFPZAM2' 'wJalrXUtnFEMI/K7MDENG/' 'bPxRfiCYzEXAMPLEKEY1' > "$WORK/leaky/config.ini"
echo 'hello = "world"' > "$WORK/clean/config.ini"
chmod -R a+rX "$WORK"

set +e
docker run --rm --read-only -v "$WORK/leaky:/scan:ro" "$IMAGE" dir /scan --no-banner
rc=$?
set -e
[ "$rc" = 1 ] || { echo "planted key not reported (exit $rc)"; exit 1; }
docker run --rm --read-only -v "$WORK/clean:/scan:ro" "$IMAGE" dir /scan --no-banner \
  || { echo "clean directory reported"; exit 1; }

# history scan: the key is committed, then deleted; gitleaks git must still find it
git -C "$WORK/leaky" init -q
git -C "$WORK/leaky" -c user.email=t@t -c user.name=t add config.ini
git -C "$WORK/leaky" -c user.email=t@t -c user.name=t commit -qm leak
git -C "$WORK/leaky" rm -q config.ini
git -C "$WORK/leaky" -c user.email=t@t -c user.name=t commit -qm clean
chmod -R a+rX "$WORK"
set +e
docker run --rm --read-only --tmpfs /tmp -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' \
  -v "$WORK/leaky:/repo:ro" "$IMAGE" git /repo --no-banner
rc=$?
set -e
[ "$rc" = 1 ] || { echo "key in history not reported (exit $rc)"; exit 1; }
echo "smoke test passed (gitleaks ${WANT:-?}, uid $user)"
