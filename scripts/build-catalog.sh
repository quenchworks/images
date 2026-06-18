#!/usr/bin/env bash
# ============================================================================
# Build the whole catalog NATIVELY on this machine, JOBS apps at a time.
# ----------------------------------------------------------------------------
# Strategy (the one we settled on): fan over apps with a fixed-size job pool —
# JOBS apps build concurrently; each app's worker runs build-all.sh <app>,
# which builds EVERY version of that app in order (multi-arch, 0-CVE gate,
# publish + cosign sign + SBOM/SLSA attest). When one app finishes, the next
# starts. Continues past failures and prints a summary.
#
# Usage:  scripts/build-catalog.sh [app ...]   # no args = whole catalog
# Env:    JOBS    apps in parallel              (default: 2)
#         PUSH    1=publish+sign  0=build+scan  (default: 1)
#         ARCHES  arches to build               (default: x86_64,aarch64)
# Prereq: `make login` first (push needs GHCR auth) and a cosign key.
# ============================================================================
set -uo pipefail   # not -e: one failing app must not stop the rest
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"; cd "$ROOT"
export PATH="$HOME/.local/bin:$PATH"

JOBS="${JOBS:-2}"
export PUSH="${PUSH:-1}"
export ARCHES="${ARCHES:-x86_64,aarch64}"
# Build temp on the big disk, not the small RAM-backed tmpfs at /tmp (parallel
# heavy melange builds overflow a 14G tmpfs). build-image.sh sets the same.
export TMPDIR="${QUENCH_TMPDIR:-/var/tmp/quench-build}"
mkdir -p "$TMPDIR"

# app list: args, else every apps/*/
APPS=()
if [ "$#" -gt 0 ]; then
  APPS=("$@")
else
  for d in apps/*/; do APPS+=("$(basename "$d")"); done
fi

STAMP="$(date +%Y-%m-%d_%H-%M-%S)"
LOGDIR="logs/$(date +%Y-%m-%d)/catalog-$STAMP"
mkdir -p "$LOGDIR"
export LOGDIR

echo "🏗  catalog build — ${#APPS[@]} apps, JOBS=$JOBS, arches=$ARCHES, push=$PUSH"
echo "📂 per-app logs in $LOGDIR/"
echo ""

# Pre-register qemu/binfmt once (build-image.sh also does this, but doing it
# up front avoids two parallel workers racing the first registration).
if [[ "$ARCHES" == *aarch64* ]] && [ "$(uname -m)" != "aarch64" ]; then
  echo "🧬 registering qemu/binfmt for aarch64 ..."
  docker run --privileged --rm tonistiigi/binfmt --install arm64 >/dev/null 2>&1 || true
fi

# Worker: build every version of one app, logging to its own file.
run_app() {
  local a="$1"
  if PUSH="$PUSH" ARCHES="$ARCHES" scripts/build-all.sh "$a" > "$LOGDIR/$a.log" 2>&1; then
    printf '✅ %s\n' "$a"
  else
    printf '❌ %s  (see %s/%s.log)\n' "$a" "$LOGDIR" "$a"
    touch "$LOGDIR/.fail.$a"
  fi
}

# Fixed-size pool: keep at most JOBS workers alive at once.
pids=()
reap() {                       # drop finished pids from the tracking array
  local alive=()
  local p
  for p in "${pids[@]}"; do
    if kill -0 "$p" 2>/dev/null; then alive+=("$p"); fi
  done
  pids=("${alive[@]}")
}

for a in "${APPS[@]}"; do
  while true; do
    reap
    [ "${#pids[@]}" -lt "$JOBS" ] && break
    sleep 2
  done
  echo "▶  start $a"
  run_app "$a" &
  pids+=("$!")
done
wait

# Summary
fail=$(find "$LOGDIR" -maxdepth 1 -name '.fail.*' 2>/dev/null | wc -l | tr -d ' ')
ok=$(( ${#APPS[@]} - fail ))
echo ""
echo "════════════════════════════════════════════"
echo "catalog done: $ok ok, $fail failed (of ${#APPS[@]} apps).  logs: $LOGDIR/"
if [ "$fail" -gt 0 ]; then
  echo "failed apps:"; find "$LOGDIR" -maxdepth 1 -name '.fail.*' -exec basename {} \; | sed 's/^\.fail\./  - /'
fi
[ "$fail" -eq 0 ]
