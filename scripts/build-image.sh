#!/usr/bin/env bash
# ============================================================================
# Local build pipeline for ONE QuenchWorks image.
# Replaces the GitHub Actions build for the (private) images repo: apko builds
# the multi-arch image (from-source apps build their package with melange
# first), Trivy enforces the 0-CVE gate, then the published index is
# cosign-signed and attested (SBOM + SLSA provenance) with a LOCAL KEY.
#
# Usage:  scripts/build-image.sh <app> [version]
# Env:
#   ARCHES        arches to build           (default: x86_64,aarch64)
#   PUSH          1=publish+sign  0=build+scan only   (default: 1)
#   COSIGN_KEY    cosign private key        (default: .secrets/cosign.key)
#   GHCR_OWNER    org                       (default: quenchworks)
# ============================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="${1:?usage: build-image.sh <app> [version]}"
VERSION_ARG="${2:-}"
ARCHES="${ARCHES:-x86_64,aarch64}"
PUSH="${PUSH:-1}"
OWNER="${GHCR_OWNER:-quenchworks}"
GHCR="ghcr.io/${OWNER}/images/${APP}"
COSIGN_KEY="${COSIGN_KEY:-$ROOT/.secrets/cosign.key}"
APPDIR="$ROOT/apps/$APP"

[ -d "$APPDIR" ]            || { echo "❌ no such app: apps/$APP"; exit 1; }
[ -f "$APPDIR/apko.yaml" ] || { echo "❌ apps/$APP/apko.yaml missing"; exit 1; }
cd "$APPDIR"

# --- version: explicit arg > melange.yaml > catalog > "latest" -------------
VERSION="$VERSION_ARG"
if [ -z "$VERSION" ] && [ -f melange.yaml ]; then
  VERSION="$(awk '/^  version:/{print $2; exit}' melange.yaml)"
fi
if [ -z "$VERSION" ]; then
  VERSION="$(awk -v a="$APP" '$0 ~ "name: "a"$"{f=1} f&&/version:/{gsub(/[",]/,"");print $2;exit}' "$ROOT/catalog.yaml" 2>/dev/null || true)"
  [ -z "$VERSION" ] && VERSION="latest"
fi

# --- guard: unrendered placeholder (node/python/etc. need a rendered apko) --
if grep -vE '^[[:space:]]*#' apko.yaml | grep -q '__[A-Z0-9_]*__'; then
  echo "❌ apps/$APP/apko.yaml has an unrendered placeholder (e.g. __NODEVER__)."
  echo "   Placeholder/runtime apps need a per-app render step; not yet supported here."
  exit 2
fi

echo "🏗  $APP:$VERSION   arches=$ARCHES   push=$PUSH"

# --- from-source apps: build the package with melange ----------------------
if [ -f melange.yaml ]; then
  command -v melange >/dev/null || { echo "❌ melange not installed"; exit 1; }
  [ -f melange.rsa ] || melange keygen melange.rsa >/dev/null 2>&1
  if [[ "$ARCHES" == *aarch64* ]] && [ "$(uname -m)" != "aarch64" ]; then
    echo "🧬 registering qemu/binfmt for aarch64 emulation ..."
    docker run --privileged --rm tonistiigi/binfmt --install arm64 >/dev/null 2>&1 || true
  fi
  echo "📦 melange build ($ARCHES) ..."
  melange build melange.yaml --arch "$ARCHES" --signing-key melange.rsa --out-dir ./packages
fi

# --- 0-CVE gate: assemble the native arch and scan the tar -----------------
NATIVE="$(uname -m)"; [ "$NATIVE" = "arm64" ] && NATIVE="aarch64"
echo "🔎 apko build (scan tar, $NATIVE) ..."
apko build apko.yaml "$GHCR:scan" image.tar --arch "$NATIVE" >/dev/null
echo "🛡  trivy 0-CVE gate ..."
trivy image --input image.tar --exit-code 1 --ignore-unfixed \
  --severity CRITICAL,HIGH,MEDIUM,LOW --scanners vuln --quiet
echo "✅ 0 fixable CVEs"

if [ "$PUSH" != "1" ]; then
  echo "⏭  PUSH=0 — built + scanned only, not publishing."
  exit 0
fi

# --- publish the multi-arch index ------------------------------------------
command -v cosign >/dev/null || { echo "❌ cosign not installed"; exit 1; }
echo "⬆  apko publish $GHCR:$VERSION ($ARCHES) ..."
apko publish apko.yaml "$GHCR:$VERSION" --arch "$ARCHES" >/dev/null
DIGEST="$(crane digest "$GHCR:$VERSION")"
REF="$GHCR@$DIGEST"
echo "   $REF"

# --- sign + attest (key-based; no OIDC locally) ----------------------------
export COSIGN_PASSWORD="${COSIGN_PASSWORD:-$(cat "$ROOT/.secrets/cosign.password" 2>/dev/null || true)}"
echo "✍  cosign sign ..."
cosign sign --key "$COSIGN_KEY" --yes "$REF" >/dev/null
echo "📄 syft SBOM + cosign attest (spdxjson) ..."
syft "$REF" -o spdx-json=sbom.spdx.json -q
cosign attest --key "$COSIGN_KEY" --yes --type spdxjson --predicate sbom.spdx.json "$REF" >/dev/null
echo "🔗 SLSA provenance attest ..."
cat > provenance.json <<JSON
{
  "buildType": "https://quench-works.com/build/local/v1",
  "builder": { "id": "https://quench-works.com/local-builder" },
  "invocation": { "configSource": { "uri": "git+https://github.com/${OWNER}/images", "entryPoint": "apps/${APP}" } },
  "metadata": { "buildStartedOn": "$(date -u +%Y-%m-%dT%H:%M:%SZ)" }
}
JSON
cosign attest --key "$COSIGN_KEY" --yes --type slsaprovenance --predicate provenance.json "$REF" >/dev/null

echo "🎉 $APP:$VERSION  published + signed + attested → $REF"
