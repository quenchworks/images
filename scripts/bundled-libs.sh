#!/usr/bin/env bash
# Lists native code a vulnerability scan cannot see inside an image's Python venvs:
# manylinux `*.libs` directories, and TLS or curl versions compiled into site-packages
# extensions. Trivy scores a wheel by its package version only, so an image can scan 0
# while a wheel carries its own OpenSSL (psycopg-binary, confluent-kafka, cryptography).
# Usage: scripts/bundled-libs.sh <image-ref>
# A version string alone is not proof of a bundled copy (Wolfi builds print the header
# version they compiled against); the NEEDED column says whether libssl is linked.
set -uo pipefail
img=${1:?usage: bundled-libs.sh <image-ref>}
d=$(mktemp -d "$HOME/.quench-bundled-XXXXXX"); c=bundled-$$
trap 'rm -rf "$d"; docker rm -f "$c" >/dev/null 2>&1' EXIT
docker pull -q --platform linux/amd64 "$img" >/dev/null || { echo "pull failed: $img"; exit 1; }
docker create --name "$c" --platform linux/amd64 "$img" >/dev/null
docker export "$c" | tar -x -C "$d" 2>/dev/null
find "$d" -type d -name '*.libs' -path '*site-packages*' | sed "s|^$d||"
find "$d" -path '*site-packages*' -name '*.so*' -type f -size +500k 2>/dev/null | while read -r f; do
  s=$(strings -n 8 "$f" | grep -oE 'OpenSSL [0-9]+\.[0-9]+\.[0-9]+[a-z]?|libcurl/[0-9.]+|BoringSSL' | sort -u | tr '\n' ' ')
  [ -n "$s" ] || continue
  if readelf -d "$f" 2>/dev/null | grep -q 'NEEDED.*libssl'; then n="links libssl"; else n="NO libssl in NEEDED"; fi
  echo "${f#$d}: $s($n)"
done
