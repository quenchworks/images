#!/usr/bin/env bash
# Smoke test for a built istio-proxyv2 image. Usage: test.sh <image-ref> [expected-version]
#
# A sidecar needs istiod and a mesh to do its real job; that is the istiod chart's gate.
# This proves the pieces are the right ones and run:
#   - the shipped envoy reports the proxy commit the image declares
#     (ISTIO_META_ISTIO_PROXY_SHA, the PROXY_REPO_SHA istio/istio pins for this tag),
#   - that envoy starts and serves its admin API as a LIVE server,
#   - pilot-agent reports the ldflag-stamped version,
#   - the bootstrap template pilot-agent renders at startup is where it looks for it.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"
NAME="quench-istio-proxyv2-smoke-$$"
cleanup() { docker rm -f "$NAME" "$NAME-cp" >/dev/null 2>&1 || true; }
trap cleanup EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

want_sha="$(docker inspect "$IMAGE" --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^ISTIO_META_ISTIO_PROXY_SHA=//p')"
[ ${#want_sha} = 40 ] || { echo "ISTIO_META_ISTIO_PROXY_SHA is not a commit: '$want_sha'"; exit 1; }
envoy_ver="$(docker run --rm --entrypoint /usr/local/bin/envoy "$IMAGE" --version 2>&1)"
echo "envoy: $envoy_ver"
case "$envoy_ver" in
  *" version: $want_sha/"*) ;;
  *) echo "envoy is not the pinned Istio proxy build $want_sha"; exit 1 ;;
esac

ver="$(docker run --rm --entrypoint /usr/local/bin/pilot-agent "$IMAGE" version -s 2>/dev/null | tr -d "\r" | sed -n "s/^client version: //p")"
echo "pilot-agent: $ver"
case "$ver" in ""|unknown*) echo "pilot-agent version not stamped"; exit 1 ;; esac
if [ -n "$EXPECT_VER" ] && [ "$ver" != "$EXPECT_VER" ]; then
  echo "expected pilot-agent $EXPECT_VER, got '$ver'"; exit 1
fi

docker create --name "$NAME-cp" "$IMAGE" >/dev/null
tmpl="$(docker cp "$NAME-cp:/var/lib/istio/envoy/envoy_bootstrap_tmpl.json" - | tar -xO)"
grep -q '"dynamic_resources"' <<<"$tmpl" || { echo "bootstrap template missing or not Istio's"; exit 1; }

docker run -d --name "$NAME" -p 127.0.0.1:15000:15000 --entrypoint /usr/local/bin/envoy "$IMAGE" \
  --config-yaml '{admin: {address: {socket_address: {address: 0.0.0.0, port_value: 15000}}}}' >/dev/null
for i in $(seq 1 30); do
  info="$(curl -fsS --max-time 3 http://127.0.0.1:15000/server_info 2>/dev/null || true)"
  [ -n "$info" ] && break
  [ "$i" = 30 ] && { echo "envoy admin never answered"; docker logs "$NAME" | tail -20; exit 1; }
  sleep 1
done
grep -q '"state": "LIVE"' <<<"$info" || { echo "envoy is not LIVE"; echo "$info" | head -20; exit 1; }
grep -q "$want_sha" <<<"$info" || { echo "server_info does not report $want_sha"; exit 1; }

echo "smoke test passed (pilot-agent $ver, envoy $want_sha LIVE, nonroot user $user)"
