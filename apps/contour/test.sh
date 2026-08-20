#!/usr/bin/env bash
# Smoke test for a built Contour image. Usage: test.sh <image-ref> [expected-version]
#
# WHY THIS SHAPE (the wave plan flags controller boot-testing as an open problem):
# A `serve`-only controller needs an apiserver, so booting it here would only prove that
# it can fail to reach one. Contour is luckier than most: `contour bootstrap <path>` is a
# FULLY OFFLINE, deterministic code path that builds the real Envoy bootstrap document
# through the same go-control-plane types the xDS server uses, marshals it, and writes the
# SDS resource files -- no cluster, no network, no Envoy. So we assert on its OUTPUT, not
# just on an exit code:
#   * envoy.json parses as JSON and its ADS config points at the xDS cluster we asked for
#   * the static `contour` cluster carries the --xds-address/--xds-port we passed
#     (i.e. flags really reached the generator; a stub that wrote a canned file would not)
#   * the admin unix socket path we passed is in the document
#   * --resources-dir produced both SDS files, which is the TLS path the chart uses
# Plus: the image runs READ-ONLY as uid 1001 (only the bind mount is writable), and the
# single binary exposes every subcommand the chart invokes for its different pod roles.
#
# NOT tested here, on purpose: whether the Envoy we ship ACCEPTS the generated bootstrap
# (`envoy --mode validate -c envoy.json`). That needs the quenchworks envoy image, which
# is not pulled at boot-test time (this step runs before the GHCR login). It was verified
# by hand while authoring -- contour 1.33.6's bootstrap validates clean on
# ghcr.io/quenchworks/images/envoy:1.38.1 -- and it belongs in the chart's kind gate,
# which has both images and a real cluster.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
EXPECT_VER="${2:-}"

WORK="$(mktemp -d)"
# the image runs as uid 1001; the bind mount must be writable by it
chmod 0777 "$WORK"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

echo "== image config"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
EP="$(docker inspect --format '{{json .Config.Entrypoint}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
echo "  Entrypoint:  $EP"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }
echo "$EP" | grep -q '/usr/bin/contour' || { echo "FAIL: entrypoint is not /usr/bin/contour"; exit 1; }

echo "== contour version"
# NB: `contour version` uses Go's builtin println, which writes to STDERR, and logrus
# also logs a maxprocs line there -- so capture both streams and filter by prefix.
VER_OUT="$(docker run --rm --read-only "$IMAGE" version 2>&1)"
echo "$VER_OUT" | sed 's/^/  /'
VER="$(echo "$VER_OUT" | sed -n 's/^version: v\{0,1\}//p')"
SHA="$(echo "$VER_OUT" | sed -n 's/^sha: //p')"
[ -n "$VER" ] || { echo "FAIL: no version line in \`contour version\`"; exit 1; }
case "$VER" in ""|0.0.0|*dev*|*unknown*) echo "FAIL: version not stamped: '$VER'"; exit 1 ;; esac
echo "$SHA" | grep -Eq '^[0-9a-f]{40}$' || { echo "FAIL: build sha not stamped: '$SHA'"; exit 1; }
if [ -n "$EXPECT_VER" ]; then
  [ "$VER" = "$EXPECT_VER" ] || { echo "FAIL: expected version $EXPECT_VER, got $VER"; exit 1; }
  echo "  version matches $EXPECT_VER"
fi

echo "== subcommand surface (one image, every chart role)"
HELP="$(docker run --rm --read-only "$IMAGE" --help 2>&1)"
for cmd in "serve" "bootstrap" "certgen" "gateway-provisioner" "envoy shutdown-manager" "envoy shutdown" "cli cds" "version"; do
  echo "$HELP" | grep -q "^${cmd}" || { echo "FAIL: subcommand '$cmd' missing"; echo "$HELP"; exit 1; }
  echo "  ok: $cmd"
done

echo "== contour bootstrap: generate a real Envoy bootstrap, offline, read-only rootfs"
# stand-in xDS client certs: bootstrap only records their PATHS, but it stat()s them first
for f in ca.crt tls.crt tls.key; do echo "not-a-real-cert" > "$WORK/$f"; done
docker run --rm --read-only -v "$WORK:/out" "$IMAGE" \
  bootstrap /out/envoy.json \
  --xds-address=contour --xds-port=8001 --xds-resource-version=v3 \
  --resources-dir=/out/resources \
  --envoy-cafile=/out/ca.crt --envoy-cert-file=/out/tls.crt --envoy-key-file=/out/tls.key \
  --admin-address=/admin/admin.sock 2>&1 | sed 's/^/  /'

for f in envoy.json resources/sds/xds-tls-certificate.json resources/sds/xds-validation-context.json; do
  [ -s "$WORK/$f" ] || { echo "FAIL: bootstrap did not write $f"; exit 1; }
  echo "  wrote $f ($(wc -c <"$WORK/$f") bytes)"
done

python3 - "$WORK" <<'PY'
import json, sys, pathlib
work = pathlib.Path(sys.argv[1])
bs = json.loads((work / "envoy.json").read_text())

# every ADS/LDS/CDS stream must point at the xDS cluster contour serves
dyn = bs["dynamic_resources"]
streams = [k for k in ("lds_config", "cds_config", "ads_config") if k in dyn]
assert streams, sorted(dyn)
for key in streams:
    src = dyn[key].get("api_config_source", dyn[key])
    grpc = src["grpc_services"][0]["envoy_grpc"]
    assert grpc["cluster_name"] == "contour", (key, grpc)
    assert src["api_type"] == "GRPC", (key, src["api_type"])

# the static cluster must carry the --xds-address/--xds-port we passed on the CLI
clusters = {c["name"]: c for c in bs["static_resources"]["clusters"]}
assert "contour" in clusters, sorted(clusters)
sock = clusters["contour"]["load_assignment"]["endpoints"][0]["lb_endpoints"][0]["endpoint"]["address"]["socket_address"]
assert sock["address"] == "contour" and sock["port_value"] == 8001, sock

# the --admin-address unix socket we passed must be the admin listener
assert bs["admin"]["address"]["pipe"]["path"] == "/admin/admin.sock", bs["admin"]["address"]

# both SDS documents must be real JSON, not empty files
for f in ("xds-tls-certificate.json", "xds-validation-context.json"):
    json.loads((work / "resources" / "sds" / f).read_text())

print("  bootstrap assertions ok: ads->contour, cluster contour:8001, admin /admin/admin.sock, 2 SDS docs")
PY

echo "PASS: contour smoke test green (version $VER, sha ${SHA:0:12}, nonroot uid $USER_CFG)"
