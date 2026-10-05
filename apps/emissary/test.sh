#!/usr/bin/env bash
# Smoke test for a built Emissary-ingress image. Usage: test.sh <image-ref> [version]
# Without a cluster the entrypoint starts its goroutines (ambex, diagd, envoy) and stops when
# the Kubernetes watcher cannot reach an API server, so this proves the pieces are wired: the
# version-stamped start line, diagd launched, Envoy 1.37 on PATH and the Python packages
# importable. Routing itself is gated by the chart's kind test.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"

env_ver="$(docker run --rm --entrypoint /usr/local/bin/envoy "$IMAGE" --version | tr -s "\n" " ")"
grep -qE '/1\.37\.[0-9]+/' <<<"$env_ver" || { echo "unexpected envoy: $env_ver"; exit 1; }
docker run --rm --entrypoint /opt/ambassador/emissary/bin/python "$IMAGE" \
  -c 'import ambassador, ambassador_diag.diagd, orjson, requests; print("python packages import")'

out="$(timeout 60 docker run --rm "$IMAGE" 2>&1 || true)"
ver="$(grep -oE 'Started Ambassador \(Version [^)]+\)' <<<"$out" | grep -oE '[0-9][^)]*' || true)"
[ -n "$ver" ] || { echo "no start line"; echo "$out" | tail -20; exit 1; }
[ -z "${2:-}" ] || [ "$ver" = "$2" ] || { echo "version $ver, expected $2"; exit 1; }
grep -q 'started command \[\\"diagd\\"' <<<"$out" || { echo "diagd was not started"; exit 1; }
grep -q 'Listening on tcp:127.0.0.1:8003' <<<"$out" || { echo "ambex did not listen"; exit 1; }
grep -q 'goroutine \\"/watcher\\" exited with error: Get \\"http://localhost:8080/api' <<<"$out" \
  || { echo "watcher did not reach for the API server"; echo "$out" | tail -20; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "8888" ] || { echo "expected user 8888, got '$user'"; exit 1; }
echo "smoke test passed (version $ver, $env_ver, user $user)"
