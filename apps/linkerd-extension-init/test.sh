#!/usr/bin/env bash
# Smoke test for a built linkerd-extension-init image. Usage: test.sh <image-ref> [version]
# Runs the real Job command against a minimal fake API server (qw fakeapi on the runner, TLS with an
# IP SAN so rustls accepts it). src/main.rs: GET the extension namespace, GET linkerd-config in the
# control-plane namespace (cniEnabled), then a JSON patch of the namespace. The fake answers those
# three calls and records the patch; the process must exit 0 and the patch must add the extension
# label and pod-security "restricted" (cniEnabled: true). Labelling a real namespace: the chart's gate.
set -euo pipefail
# qw (tools/qw, Go) provides the helpers this test needs; go run reuses the build cache.
QWD="$(cd "$(dirname "$0")/../../tools/qw" && pwd)"; qw() { go -C "$QWD" run . "$@"; }
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
D="$(mktemp -d)"; FAKE=""
cleanup() { [ -z "$FAKE" ] || kill "$FAKE" 2>/dev/null || true; rm -rf "$D"; }
trap cleanup EXIT
# the fake API: the namespace and linkerd-config for GET, and the namespace PATCH recorded to patch.json
printf '%s' '{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"linkerd-viz"}}' > "$D/ns.json"
jq -n '{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "linkerd-config", namespace: "linkerd"}, data: {values: "cniEnabled: true\n"}}' > "$D/cm.json"
go -C "$QWD" build -o "$D/qw" .
# A CA plus a leaf cert: rustls/webpki rejects a self-signed CA cert served as the end entity
# (CaUsedAsEndEntity, run 37806161800).
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj /CN=fake-ca \
  -keyout "$D/ca.key" -out "$D/ca.crt" 2>/dev/null
openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj /CN=fake-apiserver \
  -keyout "$D/api.key" -out "$D/api.csr" 2>/dev/null
printf 'subjectAltName=IP:127.0.0.1\nbasicConstraints=CA:FALSE\nextendedKeyUsage=serverAuth\n' > "$D/ext.cnf"
openssl x509 -req -in "$D/api.csr" -CA "$D/ca.crt" -CAkey "$D/ca.key" -CAcreateserial -days 1 \
  -extfile "$D/ext.cnf" -out "$D/api.crt" 2>/dev/null
"$D/qw" fakeapi --addr 127.0.0.1:16444 --cert "$D/api.crt" --key "$D/api.key" \
  --route "GET /api/v1/namespaces/linkerd-viz=200:$D/ns.json" \
  --route "GET /api/v1/namespaces/linkerd/configmaps/linkerd-config=200:$D/cm.json" \
  --route "PATCH /api/v1/namespaces/linkerd-viz=200:$D/ns.json" --record "PATCH /api/v1/namespaces/linkerd-viz=$D/patch.json" & FAKE=$!
cat > "$D/kubeconfig" <<K
apiVersion: v1
kind: Config
clusters:
- cluster: {server: "https://127.0.0.1:16444", certificate-authority-data: "$(base64 -w0 < "$D/ca.crt")"}
  name: fake
contexts:
- context: {cluster: fake, user: fake}
  name: fake
current-context: fake
users:
- name: fake
  user: {token: fake-token}
K
chmod 755 "$D"; chmod 644 "$D/kubeconfig"
for i in $(seq 1 20); do curl -sk -o /dev/null https://127.0.0.1:16444/ && break; sleep 0.5; done
rc=0
out="$(timeout 60 docker run --rm --network host --read-only -v "$D/kubeconfig:/cfg/kubeconfig:ro" -e KUBECONFIG=/cfg/kubeconfig \
  "$IMAGE" --log-format plain --log-level info --extension viz --namespace linkerd-viz --linkerd-namespace linkerd 2>&1)" || rc=$?
echo "$out"
[ "$rc" = 0 ] || { echo "extension-init exited $rc"; exit 1; }
[ -s "$D/patch.json" ] || { echo "no PATCH reached the API server"; exit 1; }
echo "patch: $(cat "$D/patch.json")"
jq -e '[.[] | select(.op == "add") | {(.path): .value}] | add
  | .["/metadata/labels/linkerd.io~1extension"] == "viz" and .["/metadata/labels/pod-security.kubernetes.io~1enforce"] == "restricted"' "$D/patch.json" >/dev/null \
  || { echo "PATCH lacks the extension and pod-security labels"; exit 1; }
grep -q 'successfully patched namespace' <<<"$out" || { echo "no success log line"; exit 1; }
echo "smoke test passed"
