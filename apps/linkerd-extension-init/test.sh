#!/usr/bin/env bash
# Smoke test for a built linkerd-extension-init image. Usage: test.sh <image-ref> [version]
# Runs the real Job command against a minimal fake API server (python3 on the runner, TLS with an
# IP SAN so rustls accepts it). src/main.rs: GET the extension namespace, GET linkerd-config in the
# control-plane namespace (cniEnabled), then a JSON patch of the namespace. The fake answers those
# three calls and records the patch; the process must exit 0 and the patch must add the extension
# label and pod-security "restricted" (cniEnabled: true). Labelling a real namespace: the chart's gate.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
D="$(mktemp -d)"; FAKE=""
cleanup() { [ -z "$FAKE" ] || kill "$FAKE" 2>/dev/null || true; rm -rf "$D"; }
trap cleanup EXIT
cat > "$D/fakeapi.py" <<'P'
import http.server, json, ssl, sys
NS = {"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": "linkerd-viz"}}
CM = {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "linkerd-config", "namespace": "linkerd"},
      "data": {"values": "cniEnabled: true\n"}}
class H(http.server.BaseHTTPRequestHandler):
    def send(self, code, body):
        b = json.dumps(body).encode()
        self.send_response(code); self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(b))); self.end_headers(); self.wfile.write(b)
    def do_GET(self):
        p = self.path.split("?")[0]
        if p == "/api/v1/namespaces/linkerd-viz": return self.send(200, NS)
        if p == "/api/v1/namespaces/linkerd/configmaps/linkerd-config": return self.send(200, CM)
        self.send(404, {"kind": "Status", "apiVersion": "v1", "status": "Failure", "code": 404})
    def do_PATCH(self):
        body = self.rfile.read(int(self.headers.get("Content-Length") or 0))
        if self.path.split("?")[0] == "/api/v1/namespaces/linkerd-viz":
            open(sys.argv[3], "wb").write(body); return self.send(200, NS)
        self.send(404, {"kind": "Status", "apiVersion": "v1", "status": "Failure", "code": 404})
    def log_message(self, *a): pass
s = http.server.ThreadingHTTPServer(("127.0.0.1", 16444), H)
c = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER); c.load_cert_chain(sys.argv[1], sys.argv[2])
s.socket = c.wrap_socket(s.socket, server_side=True); s.serve_forever()
P
# A CA plus a leaf cert: rustls/webpki rejects a self-signed CA cert served as the end entity
# (CaUsedAsEndEntity, run 37806161800).
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj /CN=fake-ca \
  -keyout "$D/ca.key" -out "$D/ca.crt" 2>/dev/null
openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj /CN=fake-apiserver \
  -keyout "$D/api.key" -out "$D/api.csr" 2>/dev/null
printf 'subjectAltName=IP:127.0.0.1\nbasicConstraints=CA:FALSE\nextendedKeyUsage=serverAuth\n' > "$D/ext.cnf"
openssl x509 -req -in "$D/api.csr" -CA "$D/ca.crt" -CAkey "$D/ca.key" -CAcreateserial -days 1 \
  -extfile "$D/ext.cnf" -out "$D/api.crt" 2>/dev/null
python3 "$D/fakeapi.py" "$D/api.crt" "$D/api.key" "$D/patch.json" & FAKE=$!
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
python3 - "$D/patch.json" <<'PY'
import json, sys
ops = json.load(open(sys.argv[1]))
got = {o["path"]: o.get("value") for o in ops if o.get("op") == "add"}
assert got.get("/metadata/labels/linkerd.io~1extension") == "viz", got
assert got.get("/metadata/labels/pod-security.kubernetes.io~1enforce") == "restricted", got
PY
grep -q 'successfully patched namespace' <<<"$out" || { echo "no success log line"; exit 1; }
echo "smoke test passed"
