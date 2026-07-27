#!/usr/bin/env bash
# Smoke test for a built Crossplane image. Usage: test.sh <image-ref>
# Crossplane only does real work against an API server (the chart's kind install gate is
# the runtime test). Here we verify the multi-call binary executes, reports a stamped
# release, exposes all three subcommands the chart drives, ships the core CRD/webhook
# trees `crossplane core init` reads from disk, and runs as nonroot.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"

echo "checking --version reports a stamped release"
ver="$(docker run --rm "$IMAGE" --version 2>&1 | tr -d '\r')"
echo "reported version: $ver"
case "$ver" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "version not stamped: '$ver'"; exit 1 ;;
esac

echo "checking the multi-call CLI exposes the subcommands the chart drives"
out="$(docker run --rm "$IMAGE" --help 2>&1 || true)"
for cmd in "core start" "core init" "rbac start"; do
  echo "$out" | grep -q "$cmd" || { echo "no '$cmd' in --help"; echo "$out"; exit 1; }
done

echo "checking core start knows its default ports"
start="$(docker run --rm "$IMAGE" core start --help 2>&1 || true)"
for flag in --webhook-port --metrics-port --health-probe-port; do
  echo "$start" | grep -q -- "$flag" || { echo "no $flag in 'core start --help'"; exit 1; }
done

echo "checking the core CRD + webhook trees ship at the paths core init defaults to"
# `core init` reads its CRDs path (default /crds, flag spelled --cr-ds-path because kong
# dasherizes the CRDsPath field name) and --webhook-configurations-path (default
# /webhookconfigurations) off the image filesystem. Missing trees mean a cluster that
# never gets its CRDs, and nothing else would notice until install time.
init="$(docker run --rm "$IMAGE" core init --help 2>&1 || true)"
echo "$init" | grep -q -- '--cr-ds-path="/crds"' \
  || { echo "core init does not default its CRDs path to /crds"; echo "$init"; exit 1; }
echo "$init" | grep -q -- '--webhook-configurations-path="/webhookconfigurations"' \
  || { echo "core init does not default its webhook path to /webhookconfigurations"; exit 1; }
id="$(docker create "$IMAGE")"
trap 'docker rm -f "$id" >/dev/null 2>&1 || true' EXIT
# Listed ONCE into a variable on purpose: `docker export | tar -t | grep -q` under
# `set -o pipefail` makes grep exit on its first match, tar takes SIGPIPE (141), and
# pipefail turns a successful match into a failed pipeline.
listing="$(docker export "$id" | tar -t 2>/dev/null)"
n_crd="$(printf '%s\n' "$listing" | grep -c '^crds/.*\.yaml$' || true)"
n_whc="$(printf '%s\n' "$listing" | grep -c '^webhookconfigurations/.*\.yaml$' || true)"
echo "  /crds: $n_crd yaml, /webhookconfigurations: $n_whc yaml"
[ "$n_crd" -ge 15 ] || { echo "expected >=15 core CRDs at /crds, got $n_crd"; exit 1; }
[ "$n_whc" -ge 1 ]  || { echo "expected >=1 webhook configuration, got $n_whc"; exit 1; }
for f in pkg.crossplane.io_providers pkg.crossplane.io_functions \
         apiextensions.crossplane.io_compositeresourcedefinitions ; do
  printf '%s\n' "$listing" | grep -qx "crds/${f}.yaml" \
    || { echo "core CRD ${f}.yaml missing from /crds"; exit 1; }
done

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
echo "smoke test passed (version $ver, $n_crd core CRDs, nonroot user: $user)"
