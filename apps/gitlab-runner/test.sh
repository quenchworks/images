#!/usr/bin/env bash
# Smoke test for a built GitLab Runner image. Usage: test.sh <image-ref> [version]
# Checks the stamped version, then starts `gitlab-runner run` with a config that
# has no runners and a metrics listener: the process must come up and serve
# /metrics with gitlab_runner_version_info for this version. Registering against
# a GitLab instance is the chart's job.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-gitlab-runner-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm --entrypoint /usr/bin/gitlab-runner "$IMAGE" --version)"
echo "$ver" | head -3
[ -z "$WANT" ] || grep -qE "^Version: +${WANT}$" <<<"$ver" || { echo "expected version $WANT"; exit 1; }

printf 'concurrent = 1\nlisten_address = ":9252"\n' > "$WORK/config.toml"
chmod -R a+rX "$WORK"
docker run -d --name "$NAME" -p 127.0.0.1:9252:9252 -v "$WORK/config.toml:/home/gitlab-runner/config.toml:ro" "$IMAGE" >/dev/null
for i in $(seq 1 30); do
  m="$(curl -fsS http://127.0.0.1:9252/metrics 2>/dev/null || true)"
  grep -q '^gitlab_runner_version_info' <<<"$m" && break
  [ "$i" = 30 ] && { echo "no metrics from gitlab-runner run"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
grep '^gitlab_runner_version_info' <<<"$m" | head -1
[ -z "$WANT" ] || grep -q "version=\"${WANT}\"" <<<"$m" || { echo "metrics report another version"; exit 1; }

echo "smoke test passed (${WANT:-?}: run loop up, metrics served; nonroot user: $user)"
