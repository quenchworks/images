#!/usr/bin/env bash
# Smoke test for a built Polaris image. Usage: test.sh <image-ref>
# Audits a deliberately weak Deployment offline (CLI and dashboard), no cluster needed.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-polaris-smoke-$$"
DIR="$(mktemp -d)"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$DIR"; }
trap cleanup EXIT

cat > "$DIR/deploy.yaml" <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata: {name: weak, namespace: smoke}
spec:
  selector: {matchLabels: {app: weak}}
  template:
    metadata: {labels: {app: weak}}
    spec:
      containers:
        - name: app
          image: nginx:latest
          securityContext: {privileged: true}
YAML
chmod -R a+rX "$DIR"

# CLI: the privileged, :latest, probe-less container must fail these checks
out="$(docker run --rm -v "$DIR:/audit:ro" "$IMAGE" audit --audit-path /audit --format json)"
for check in runAsPrivileged tagNotSpecified livenessProbeMissing; do
  python3 -c 'import json,sys
d=json.loads(sys.argv[1]); c=sys.argv[2]
r=[x for x in d["Results"] if x["Name"]=="weak"][0]
res=r["PodResult"]["ContainerResults"][0]["Results"]
assert c in res and res[c]["Success"] is False, (c, res.get(c))' "$out" "$check" \
    || { echo "audit did not flag $check"; exit 1; }
done

# dashboard: serves /health and the same audit at /results.json
docker run -d --name "$NAME" -p 127.0.0.1:8080:8080 -v "$DIR:/audit:ro" "$IMAGE" \
  dashboard --port 8080 --audit-path /audit >/dev/null
for i in $(seq 1 30); do
  curl -fsS http://127.0.0.1:8080/health >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { echo "dashboard did not come up"; docker logs "$NAME"; exit 1; }
  sleep 1
done
res="$(curl -fsS http://127.0.0.1:8080/results.json)"
grep -q '"Name":"weak"' <<<"$res" || { echo "dashboard results missing the workload"; exit 1; }
# capture first: `curl | grep -q` fails under pipefail once grep exits early (SIGPIPE)
page="$(curl -fsS http://127.0.0.1:8080/)"
grep -q "<title>Fairwinds Polaris</title>" <<<"$page" || { echo "dashboard UI missing"; exit 1; }

ver="$(docker run --rm "$IMAGE" version 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
echo "reported version: $ver"
[ -n "$ver" ] || { echo "version not stamped"; exit 1; }
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (version $ver, user $user)"
