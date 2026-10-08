#!/usr/bin/env bash
# Smoke test for a built kubescape image. Usage: test.sh <image-ref> [version]
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
out="$(docker run --rm "$IMAGE" version 2>&1 || true)"
echo "kubescape version: $out"
[ -z "$WANT" ] || grep -q "v$WANT" <<<"$out" || { echo "expected v$WANT"; exit 1; }
# a real subcommand resolves, not just the root help
docker run --rm "$IMAGE" scan --help >/dev/null 2>&1 || { echo "kubescape scan --help failed"; exit 1; }
# A real offline-capable scan of a manifest: exercises the scanner and its otel setup, which a
# floated otel train can break at startup (schema URL conflict) while `version` still works.
d="$(mktemp -d)"; trap 'rm -rf "$d"' EXIT
cat > "$d/deploy.yaml" <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata: { name: demo }
spec:
  selector: { matchLabels: { app: demo } }
  template:
    metadata: { labels: { app: demo } }
    spec:
      containers:
        - { name: demo, image: nginx, securityContext: { privileged: true } }
YAML
chmod 0755 "$d"; chmod 0644 "$d/deploy.yaml"
scan="$(docker run --rm -e HOME=/tmp --tmpfs /tmp -v "$d:/work:ro" "$IMAGE" scan /work/deploy.yaml --format json 2>&1 || true)"
echo "$scan" | tail -c 600
grep -q '"controlID"' <<<"$scan" || { echo "kubescape scan produced no control results"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed"
