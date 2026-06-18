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

# --- per-app build config (uniform "pass the version from outside") ---------
# apps/<app>/build.conf may declare:  VERSIONS=(...)  and a  render <version>
# function that prints a sed program substituting that app's placeholders.
HAS_CONF=0
if [ -f build.conf ]; then
  # shellcheck disable=SC1091
  source ./build.conf
  HAS_CONF=1
fi

# --- version: explicit arg > build.conf newest > melange.yaml > catalog > latest
VERSION="$VERSION_ARG"
if [ -z "$VERSION" ] && [ "$HAS_CONF" = 1 ] && [ "${#VERSIONS[@]}" -gt 0 ]; then
  VERSION="${VERSIONS[$(( ${#VERSIONS[@]} - 1 ))]}"     # newest = last in the list
fi
if [ -z "$VERSION" ] && [ -f melange.yaml ]; then
  VERSION="$(awk '/^  version:/{print $2; exit}' melange.yaml)"
fi
if [ -z "$VERSION" ]; then
  VERSION="$(awk -v a="$APP" '$0 ~ "name: "a"$"{f=1} f&&/version:/{gsub(/[",]/,"");print $2;exit}' "$ROOT/catalog.yaml" 2>/dev/null || true)"
  [ -z "$VERSION" ] && VERSION="latest"
fi

# --- render placeholders, if any, via the app's render() --------------------
APKO=apko.yaml
MEL=melange.yaml
needs_render() { grep -vE '^[[:space:]]*#' "$1" 2>/dev/null | grep -q '__[A-Z0-9_]*__'; }
if needs_render apko.yaml || { [ -f melange.yaml ] && needs_render melange.yaml; }; then
  if [ "$HAS_CONF" = 1 ] && declare -F render >/dev/null; then
    SED_PROG="$(render "$VERSION")"
    sed -e "$SED_PROG" apko.yaml > apko.rendered.yaml; APKO=apko.rendered.yaml
    [ -f melange.yaml ] && { sed -e "$SED_PROG" melange.yaml > melange.rendered.yaml; MEL=melange.rendered.yaml; }
  else
    echo "❌ apps/$APP has placeholders (e.g. __VER__) but no build.conf render() — add apps/$APP/build.conf."
    exit 2
  fi
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
  melange build "$MEL" --arch "$ARCHES" --signing-key melange.rsa --out-dir ./packages
fi

# --- 0-CVE gate: assemble the native arch and scan the tar -----------------
NATIVE="$(uname -m)"; [ "$NATIVE" = "arm64" ] && NATIVE="aarch64"
echo "🔎 apko build (scan tar, $NATIVE) ..."
apko build "$APKO" "$GHCR:scan" image.tar --arch "$NATIVE" >/dev/null
echo "🛡  trivy 0-CVE gate ..."
trivy image --input image.tar --exit-code 1 --ignore-unfixed \
  --severity CRITICAL,HIGH,MEDIUM,LOW --scanners vuln --quiet
echo "✅ 0 fixable CVEs"

if [ "$PUSH" != "1" ]; then
  echo "⏭  PUSH=0 — built + scanned only, not publishing."
  exit 0
fi

# --- publish: one image per arch, then merge into a multi-arch manifest -----
# Mirrors the old GitHub Actions flow: each arch is published to its own tag
# (:VERSION-amd64, :VERSION-arm64), then `crane index append` merges them into
# the version tag (:VERSION) as a single multi-platform manifest. apko publish
# is pure assembly, so per-arch publish needs no qemu (only melange did).
command -v cosign >/dev/null || { echo "❌ cosign not installed"; exit 1; }
command -v crane  >/dev/null || { echo "❌ crane not installed"; exit 1; }

arch_suffix() { case "$1" in x86_64) echo amd64;; aarch64) echo arm64;; *) echo "$1";; esac; }

IFS=',' read -ra ARCH_LIST <<< "$ARCHES"
PER_ARCH=()
for a in "${ARCH_LIST[@]}"; do
  [ -z "$a" ] && continue
  tag="$GHCR:$VERSION-$(arch_suffix "$a")"
  echo "⬆  apko publish $tag ($a) ..."
  apko publish "$APKO" "$tag" --arch "$a" >/dev/null
  PER_ARCH+=("$tag")
done

echo "🧩 crane index append → $GHCR:$VERSION (merged multi-arch manifest) ..."
MERGE_ARGS=()
for r in "${PER_ARCH[@]}"; do MERGE_ARGS+=( -m "$r" ); done
crane index append "${MERGE_ARGS[@]}" -t "$GHCR:$VERSION" >/dev/null

DIGEST="$(crane digest "$GHCR:$VERSION")"
REF="$GHCR@$DIGEST"
echo "   merged: $REF"
echo "   per-arch: ${PER_ARCH[*]}"

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
