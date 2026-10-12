#!/usr/bin/env bash
# Smoke test for a built trufflehog image. Usage: test.sh <image-ref> [version]
# Real scanning, offline (--no-verification): a directory with a planted GitHub token must
# be reported (--fail exits 183) and a clean one must pass (exit 0); `trufflehog git` must
# find a token committed into history and then deleted, which runs the git client.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$ver"
[ -z "$WANT" ] || grep -qx "trufflehog $WANT" <<<"$ver" || { echo "expected version $WANT"; exit 1; }

WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/leaky" "$WORK/clean"
# a made-up classic GitHub PAT, split so this file does not trip scanners itself
printf 'token = %s%s\n' 'ghp_' 'Xk9Lm2Qp7Rt4Vw8Yz1Bc3Df5Gh6Jn0Ps2Tu4' > "$WORK/leaky/config.ini"
echo 'hello = "world"' > "$WORK/clean/config.ini"
chmod -R a+rX "$WORK"

run() { docker run --rm --read-only --tmpfs /tmp "$@"; }
set +e
out="$(run -v "$WORK/leaky:/scan:ro" "$IMAGE" filesystem /scan --no-verification --json --fail 2>/dev/null)"
rc=$?
set -e
echo "$out"
[ "$rc" = 183 ] || { echo "planted token not reported (exit $rc)"; exit 1; }
grep -q '"DetectorName":"Github"' <<<"$out" || { echo "no Github finding in output"; exit 1; }
run -v "$WORK/clean:/scan:ro" "$IMAGE" filesystem /scan --no-verification --json --fail \
  || { echo "clean directory reported"; exit 1; }

# history scan: the token is committed, then deleted; trufflehog git must still find it
g() { git -C "$WORK/leaky" -c user.email=t@t -c user.name=t "$@"; }
g init -q
g add config.ini
g commit -qm leak
g rm -q config.ini
g commit -qm clean
chmod -R a+rX "$WORK"
set +e
out="$(run -v "$WORK/leaky:/repo:ro" "$IMAGE" git file:///repo --no-verification --json --fail 2>/dev/null)"
rc=$?
set -e
echo "$out"
[ "$rc" = 183 ] || { echo "token in history not reported (exit $rc)"; exit 1; }
grep -q '"DetectorName":"Github"' <<<"$out" || { echo "no Github finding in git history output"; exit 1; }
echo "smoke test passed (trufflehog ${WANT:-?}, uid $user)"
