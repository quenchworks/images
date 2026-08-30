#!/usr/bin/env bash
# Smoke test for a built metrics-server image. Usage: test.sh <image-ref> [expected-version]
#
# WHAT THIS CAN AND CANNOT PROVE -- read before adding assertions.
#
# metrics-server resolves its delegated-authentication kubeconfig BEFORE it builds the
# apiserver's openapi spec. Outside a cluster it therefore always exits at that first
# step, printing a cobra usage dump and a goroutine trace. Both a WORKING and a BROKEN
# image reach exactly the same point here -- verified by diffing 0.9.0 against the
# published-and-broken 0.8.1: identical output up to the kubeconfig error.
#
# So a docker-only test CANNOT catch the class of regression that shipped in 0.8.1: a CVE
# bump floated k8s.io/* from v0.33.7 to v0.35.3 without regenerating
# pkg/api/generated/openapi, and the server died with
#   F openapi.go:43] Failed to build open api spec for root: cannot find model definition
#     for io.k8s.apimachinery.pkg.version.Info
# only once it had a cluster to talk to. The old test ran `--version` and passed happily
# on an image that had never served a single request.
#
# THE GUARD FOR THAT IS THE CHART'S KIND GATE, which asserts the APIService reaches
# Available=True and `kubectl top nodes` returns real numbers. Do not weaken it, and do
# not add assertions here that pretend to cover it -- an unreachable assertion is worse
# than an absent one, because it reads as coverage.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

echo "metrics-server --version:"
out="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "  $out"
grep -qiE 'v?[0-9]+\.[0-9]+\.[0-9]+' <<<"$out" || { echo "FAIL: version not stamped"; exit 1; }
if [ -n "$EXPECT_VER" ]; then
  grep -qE "v?${EXPECT_VER}" <<<"$out" \
    || { echo "FAIL: expected version ${EXPECT_VER}, got: $out"; exit 1; }
  echo "  version matches ${EXPECT_VER}"
fi

echo "starting the server (it must reach serving setup, then exit on the missing cluster):"
boot="$(docker run --rm "$IMAGE" \
          --secure-port=4443 --cert-dir=/tmp --kubelet-insecure-tls 2>&1 || true)"

# Serving setup completed: TLS material was generated. A binary that cannot get this far
# is broken in a way this test CAN see.
grep -q 'Generated self-signed cert' <<<"$boot" \
  || { echo "FAIL: never reached serving setup"; echo "$boot" | head -20; exit 1; }
echo "  serving setup OK (self-signed cert generated)"

# It must stop for the RIGHT reason. Any other early exit is a real defect.
grep -q 'unable to load in-cluster configuration' <<<"$boot" \
  || { echo "FAIL: exited before the in-cluster config step, i.e. for the wrong reason:"
       echo "$boot" | head -20; exit 1; }
echo "  exits on the missing in-cluster config, as expected outside a cluster"

# Cheap insurance only: if a future binary ever DOES reach openapi in this context, a
# failure there must be loud. It is unreachable today -- see the header.
if grep -qiE 'failed to build open ?api spec|cannot find model definition' <<<"$boot"; then
  echo "FAIL: openapi spec build failed -- generated openapi no longer matches apimachinery"
  grep -iE 'openapi|model definition' <<<"$boot" | head -5; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "FAIL: expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (nonroot user: $user). NOTE: serving is proven by the CHART's"
echo "kind gate (APIService Available + kubectl top), which this test cannot replace."
