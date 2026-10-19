#!/usr/bin/env bash
# Smoke test for a built camel-k image. Usage: test.sh <image-ref> [version]
# kamel must report the expected version, the Maven toolchain the operator drives (MAVEN_CMD
# and the mvnw shim) must run on the bundled JDK 17 under a read-only root, and
# `kamel operator` given a kubeconfig for an API server that does not answer must start,
# dial that server and not panic. The chart gate brings an IntegrationPlatform to Ready in kind.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
RUN=(docker run --rm --read-only --tmpfs /tmp:exec --tmpfs /etc/maven/m2:uid=1001)

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "== kamel version =="
v="$("${RUN[@]}" "$IMAGE" version 2>&1)"
echo "$v"
[ -z "$WANT" ] || grep -q "$WANT" <<<"$v" || { echo "expected version $WANT"; exit 1; }

echo "== \$MAVEN_CMD -version runs Maven 3.9 on JDK 17 =="
m="$("${RUN[@]}" --entrypoint /bin/sh "$IMAGE" -c '$MAVEN_CMD -version' 2>/dev/null)"
echo "$m"
grep -q 'Apache Maven 3\.9\.' <<<"$m" || { echo "no Maven 3.9"; exit 1; }
grep -q 'Java version: 17\.' <<<"$m" || { echo "Maven is not on JDK 17"; exit 1; }

echo "== the mvnw shim the operator copies into build dirs (cp --recursive) =="
w="$("${RUN[@]}" --entrypoint /bin/sh "$IMAGE" -c \
  'mkdir /tmp/b && cd /tmp/b && cp --recursive /usr/share/maven/mvnw/. . && ./mvnw -version' 2>/dev/null)"
grep -q 'Apache Maven 3\.9\.' <<<"$w" || { echo "mvnw shim failed: $w"; exit 1; }

echo "== /etc/maven/m2 is writable by the runtime user without a mount =="
docker run --rm --entrypoint /bin/sh "$IMAGE" -c 'touch /etc/maven/m2/x' \
  || { echo "/etc/maven/m2 not writable"; exit 1; }

echo "== kamel operator against a dead API server =="
cat > "$WORK/kubeconfig" <<'KC'
apiVersion: v1
kind: Config
clusters: [{name: none, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: none, user: {token: none}}]
contexts: [{name: none, context: {cluster: none, user: none}}]
current-context: none
KC
chmod -R a+rX "$WORK"
out="$(timeout 60 "${RUN[@]}" -v "$WORK/kubeconfig:/etc/kubeconfig:ro" -e KUBECONFIG=/etc/kubeconfig \
  -e WATCH_NAMESPACE= -e NAMESPACE=camel-k -e POD_NAME=camel-k-operator -e OPERATOR_ID=camel-k \
  "$IMAGE" operator 2>&1 || true)"
echo "$out" | tail -15
grep -q '127.0.0.1:1' <<<"$out" || { echo "never dialed the configured API server"; exit 1; }
if grep -qE 'panic:|read-only file system' <<<"$out"; then echo "operator crashed or wrote to the root"; exit 1; fi
echo "smoke test passed (camel-k ${WANT:-?}, uid $user, Maven on JDK 17, operator dialed the API server)"
