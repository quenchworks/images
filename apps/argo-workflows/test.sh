#!/usr/bin/env bash
# Smoke test for a built Argo Workflows image. Usage: test.sh <image-ref>
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"

# The default entrypoint is /usr/bin/workflow-controller, so exercising the
# `argo` binary needs an explicit --entrypoint override.
echo "argo version:"
out="$(docker run --rm --entrypoint /usr/bin/argo "$IMAGE" version 2>&1)"
echo "$out"
# version must be stamped from the tag (not the default v0.0.0).
echo "$out" | grep -qiE 'argo:[[:space:]]*v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "argo version not stamped"; exit 1; }

echo "workflow-controller version:"
wc_out="$(docker run --rm "$IMAGE" version 2>&1)"
echo "$wc_out"
echo "$wc_out" | grep -qiE 'v?[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "workflow-controller version not stamped"; exit 1; }

# argoexec must be present AND resolvable through $PATH: the controller injects
# it into every workflow pod as `argoexec init` / `argoexec wait`, and the
# emissary executor finds it with exec.LookPath("argoexec"). Ship the image
# without it and every workflow pod fails with
# `exec: "argoexec": executable file not found in $PATH`.
echo "argoexec version (resolved via PATH):"
ex_out="$(docker run --rm --entrypoint argoexec "$IMAGE" version 2>&1)"
echo "$ex_out"
grep -qiE 'v?[0-9]+\.[0-9]+\.[0-9]+' <<<"$ex_out" \
  || { echo "argoexec version not stamped"; exit 1; }
# it must also expose the executor subcommands the controller invokes.
ex_help="$(docker run --rm --entrypoint argoexec "$IMAGE" --help 2>&1)"
for sub in init wait emissary; do
  grep -qE "^[[:space:]]+${sub}([[:space:]]|$)" <<<"$ex_help" \
    || { echo "argoexec is missing the '${sub}' subcommand"; exit 1; }
done

# must run as the nonroot argo user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (nonroot user: $user)"
