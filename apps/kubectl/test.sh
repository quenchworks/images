#!/usr/bin/env bash
# Smoke test for a built kubectl image. Usage: test.sh <image-ref>
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"

echo "kubectl client version (stamped, not a dev build)"
ver="$(docker run --rm "$IMAGE" version --client -o json 2>/dev/null | jq -r .clientVersion.gitVersion)"
echo "  kubectl: $ver"
case "$ver" in ""|*v0.0.0*|*dirty*) echo "version not stamped: '$ver'"; exit 1 ;; esac

echo "toolbelt present"
docker run --rm --entrypoint helm "$IMAGE" version --short >/dev/null || { echo "helm missing"; exit 1; }
docker run --rm --entrypoint kustomize "$IMAGE" version >/dev/null || { echo "kustomize missing"; exit 1; }
docker run --rm --entrypoint sh "$IMAGE" -c 'command -v jq >/dev/null' || { echo "jq missing"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (kubectl $ver, toolbelt ok, nonroot $user)"
