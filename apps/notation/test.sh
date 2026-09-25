#!/usr/bin/env bash
# Smoke test for a built notation image. Usage: test.sh <image-ref> [version]
# Real signing work: push an artifact (the QuenchWorks oras image) to a throwaway
# registry (the QuenchWorks distribution image), generate a test key, sign the
# artifact by digest, and verify it against a trust policy. A second artifact that was
# never signed must fail verification.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
REG="notation-smoke-reg-$$"; NET="notation-smoke-$$"
cleanup() { docker rm -f "$REG" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$ver"
[ -z "$WANT" ] || echo "$ver" | grep -q "Version: *v${WANT}" || { echo "expected version $WANT"; exit 1; }

docker network create "$NET" >/dev/null
docker run -d --name "$REG" --network "$NET" --tmpfs /var/lib/registry:uid=1001,gid=1001 \
  ghcr.io/quenchworks/images/distribution:3.1.2 >/dev/null
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"; cleanup' EXIT
mkdir -p "$WORK/home" "$WORK/data"
echo "signed payload" > "$WORK/data/a.txt"; echo "unsigned payload" > "$WORK/data/b.txt"
chmod -R a+rwX "$WORK"

oras() { docker run --rm --network "$NET" --read-only --tmpfs /tmp --tmpfs /home/nonroot:uid=1001,gid=1001 -v "$WORK/data:/work" -w /work ghcr.io/quenchworks/images/oras:1.3.4 "$@"; }
nt() { docker run --rm --network "$NET" --read-only --tmpfs /tmp -v "$WORK/home:/home/nonroot" "$IMAGE" "$@"; }
ok=0
for _ in $(seq 1 30); do
  oras push --plain-http "$REG:5000/smoke/a:v1" a.txt >/dev/null 2>&1 && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "artifact push failed"; exit 1; }
oras push --plain-http "$REG:5000/smoke/b:v1" b.txt >/dev/null
A="$REG:5000/smoke/a@$(oras resolve --plain-http "$REG:5000/smoke/a:v1")"
B="$REG:5000/smoke/b@$(oras resolve --plain-http "$REG:5000/smoke/b:v1")"

nt cert generate-test --default quench.smoke >/dev/null
nt sign --insecure-registry --key quench.smoke "$A"
cat > "$WORK/home/policy.json" <<'POL'
{"version":"1.0","trustPolicies":[{"name":"smoke","registryScopes":["*"],
 "signatureVerification":{"level":"strict"},
 "trustStores":["ca:quench.smoke"],"trustedIdentities":["*"]}]}
POL
chmod a+r "$WORK/home/policy.json"
nt policy import --force /home/nonroot/policy.json >/dev/null
nt verify --insecure-registry "$A" | tee "$WORK/verify.log"
grep -q 'Successfully verified' "$WORK/verify.log" || { echo "signed artifact did not verify"; exit 1; }
if nt verify --insecure-registry "$B" >/dev/null 2>&1; then echo "unsigned artifact verified"; exit 1; fi
echo "smoke test passed (notation ${WANT:-?}, uid $user, sign + verify, unsigned rejected)"
