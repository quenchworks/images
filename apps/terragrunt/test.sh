#!/usr/bin/env bash
# Smoke test for a built terragrunt image. Usage: test.sh <image-ref> [version]
# Runs a real terragrunt apply: a unit whose tofu module has no providers (locals and
# outputs only), so it needs no network, then reads the output back through terragrunt.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version)"
echo "$ver"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$ver" || { echo "expected v$WANT"; exit 1; }

mkdir -p "$WORK/unit"
cat > "$WORK/unit/terragrunt.hcl" <<'HCL'
inputs = {
  greeting = "hello from terragrunt"
}
HCL
cat > "$WORK/unit/main.tf" <<'TF'
variable "greeting" { type = string }
output "message" { value = upper(var.greeting) }
TF
chmod -R a+rwX "$WORK"
out="$(docker run --rm -v "$WORK/unit:/work" -e TG_NON_INTERACTIVE=true -e TERRAGRUNT_NON_INTERACTIVE=true \
  "$IMAGE" apply -auto-approve 2>&1)" || { echo "terragrunt apply failed:"; tail -30 <<<"$out"; exit 1; }
grep -q 'Apply complete' <<<"$out" || { echo "no 'Apply complete':"; tail -30 <<<"$out"; exit 1; }
msg="$(docker run --rm -v "$WORK/unit:/work" "$IMAGE" output -raw message 2>/dev/null)"
[ "$msg" = "HELLO FROM TERRAGRUNT" ] || { echo "output was '$msg'"; exit 1; }
grep -qi 'opentofu\|tofu' <<<"$(docker run --rm --entrypoint tofu "$IMAGE" version)" || { echo "tofu missing"; exit 1; }

echo "smoke test passed (terragrunt ${WANT:-?}: apply + output through tofu; user $user)"
