#!/usr/bin/env bash
# Smoke test for a built checkov image. Usage: test.sh <image-ref> [version]
# Real scanning on a read-only root: a public-read S3 ACL set through a Terraform
# variable must fail CKV_AWS_20 and a private one must pass (variable rendering runs on
# the patched asteval), and a privileged Kubernetes Pod must fail CKV_K8S_16.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "checkov $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }

WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/public" "$WORK/private" "$WORK/k8s"
tf() { printf 'variable "acl" {\n  default = "%s"\n}\n\nresource "aws_s3_bucket" "b" {\n  bucket = "x"\n  acl    = var.acl\n}\n' "$1"; }
tf public-read > "$WORK/public/main.tf"; tf private > "$WORK/private/main.tf"
cat > "$WORK/k8s/pod.yaml" <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  containers:
    - name: c
      image: busybox
      securityContext:
        privileged: true
YAML
chmod -R a+rX "$WORK"
run() { docker run --rm --read-only --tmpfs /tmp --tmpfs /home/nonroot:uid=1001,gid=1001 -v "$WORK:/src:ro" "$IMAGE" "$@"; }
summary() { python3 -c 'import json,sys; d=json.load(sys.stdin); d=d[0] if isinstance(d,list) else d; s=d["summary"]; print(s["failed"], s["passed"])'; }

set +e
pub="$(run -d /src/public --framework terraform --check CKV_AWS_20 -o json 2>/dev/null | summary)"
priv="$(run -d /src/private --framework terraform --check CKV_AWS_20 -o json 2>/dev/null | summary)"
k8s="$(run -d /src/k8s --framework kubernetes --check CKV_K8S_16 -o json 2>/dev/null | summary)"
set -e
echo "public-read: $pub | private: $priv | privileged pod: $k8s"
[ "$pub" = "1 0" ] || { echo "public-read ACL was not flagged"; exit 1; }
[ "$priv" = "0 1" ] || { echo "private ACL did not pass"; exit 1; }
[ "$k8s" = "1 0" ] || { echo "privileged pod was not flagged"; exit 1; }
echo "smoke test passed (checkov ${WANT:-?}, uid $user, terraform variable rendering + kubernetes)"
