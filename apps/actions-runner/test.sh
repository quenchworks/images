#!/usr/bin/env bash
# Smoke test for a built actions-runner image. Usage: test.sh <image-ref> [version]
# No GitHub registration here (that needs a live token), so it proves what a job
# needs locally:
#   * the self-contained .NET runtime starts and reports the runner version;
#   * JavaScript actions get Wolfi's Node 24, through both node24 and node20;
#   * git and GNU tar are present for checkout/cache/artifact actions;
#   * the entrypoint refuses clearly when neither ARC nor RUNNER_URL/TOKEN is set.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
run() { docker run --rm --entrypoint "$1" "$IMAGE" "${@:2}"; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

v="$(run /home/runner/bin/Runner.Listener --version | tr -d '\r')"
echo "runner version: $v"
[ -n "$v" ] || { echo "Runner.Listener printed no version"; exit 1; }
[ -z "$WANT" ] || [ "$v" = "$WANT" ] || { echo "expected $WANT"; exit 1; }

for n in node24 node20; do
  nv="$(run /home/runner/externals/$n/bin/node -e 'process.stdout.write(process.version)')"
  echo "externals/$n: node $nv"
  case "$nv" in v24.*) ;; *) echo "externals/$n does not run Node 24"; exit 1;; esac
done
run /usr/bin/git --version
run /usr/bin/tar --version | head -1 | grep -q 'GNU tar' || { echo "GNU tar missing"; exit 1; }

out="$(docker run --rm "$IMAGE" 2>&1 || true)"
grep -q 'set RUNNER_URL' <<<"$out" || { echo "entrypoint did not explain the missing registration:"; echo "$out"; exit 1; }

echo "smoke test passed (runner $v, Node 24 for node24 and node20 actions, nonroot user: $user)"
