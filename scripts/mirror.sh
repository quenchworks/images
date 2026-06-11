#!/usr/bin/env bash
# Mirror a signed image digest from GHCR to Docker Hub, carrying the signature.
# Usage: mirror.sh <app> <sha256:...>
set -euo pipefail

APP="${1:?usage: mirror.sh <app> <sha256:...>}"
SHA="${2:?usage: mirror.sh <app> <sha256:...>}"

SRC="ghcr.io/quenchworks/images/${APP}@${SHA}"
DST="docker.io/quenchworks/${APP}@${SHA}"

echo "copying ${SRC} -> ${DST}"
cosign copy "${SRC}" "${DST}"
echo "done"
