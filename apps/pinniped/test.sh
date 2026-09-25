#!/usr/bin/env bash
# Smoke test for a built Pinniped image. Usage: test.sh <image-ref> [version]
# Every Pinniped server needs a Kubernetes API, so the image test proves what can be
# proved without one: the CLI reports the tag, the multi-call binary dispatches on its
# name, and the Concierge parses a full config and its downward API volume and gets as
# far as asking for in-cluster credentials. The chart's kind gate covers the rest.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
DIR="$(mktemp -d)"
trap 'rm -rf "$DIR"' EXIT

ver="$(docker run --rm --entrypoint /usr/local/bin/pinniped "$IMAGE" version 2>&1 | tail -1)"
echo "pinniped CLI reports: $ver"
case "$ver" in ""|*dev*|v0.0.0) echo "version not stamped: '$ver'"; exit 1 ;; esac
[ -z "$WANT" ] || [ "$ver" = "v$WANT" ] || { echo "expected v$WANT"; exit 1; }

# the bare server name is refused and the error lists the roles it serves
out="$(docker run --rm "$IMAGE" 2>&1 || true)"
grep -q "pinniped-concierge pinniped-supervisor" <<<"$out" || { echo "dispatch error missing roles: $out"; exit 1; }
# each role name dispatches (the supervisor takes positional paths and the
# authenticator goes straight to its in-cluster client, so neither has a usable --help;
# being accepted by the dispatcher is what the name proves)
for role in pinniped-concierge pinniped-supervisor local-user-authenticator; do
  out="$(docker run --rm --entrypoint "/usr/local/bin/$role" "$IMAGE" 2>&1 || true)"
  ! grep -q "must be invoked as one of" <<<"$out" || { echo "$role not dispatched: $out"; exit 1; }
done

mkdir -p "$DIR/podinfo"
cat > "$DIR/pinniped.yaml" <<'YAML'
apiGroupSuffix: pinniped.dev
names:
  servingCertificateSecret: p-api-tls
  credentialIssuer: p-config
  apiService: p-api
  impersonationLoadBalancerService: p-ilb
  impersonationClusterIPService: p-icip
  impersonationTLSCertificateSecret: p-itls
  impersonationCACertificateSecret: p-ica
  impersonationSignerSecret: p-isig
  agentServiceAccount: p-agent
  impersonationProxyServiceAccount: p-ipsa
  impersonationProxyLegacySecret: p-ipls
labels: {"app":"p"}
kubeCertAgent:
  namePrefix: p-kube-cert-agent-
  image: example/image@sha256:0000000000000000000000000000000000000000000000000000000000000000
YAML
printf 'app="p"\n' > "$DIR/podinfo/labels"; printf 'pinniped' > "$DIR/podinfo/namespace"
printf 'p-0' > "$DIR/podinfo/name"; printf 'uid' > "$DIR/podinfo/uid"
chmod -R a+rX "$DIR"
out="$(docker run --rm -v "$DIR:/etc/p:ro" --entrypoint /usr/local/bin/pinniped-concierge "$IMAGE" \
  --config /etc/p/pinniped.yaml --downward-api-path /etc/p/podinfo 2>&1 || true)"
grep -q "could not load in-cluster configuration" <<<"$out" \
  || { echo "concierge did not get past its config: $out"; exit 1; }
echo "  concierge loaded its config and downward API, stopped at in-cluster credentials"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user, multi-call dispatch, concierge config load)"
