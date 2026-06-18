#!/usr/bin/env bash
# Build EVERY version of every app (or one app, if a name is passed), one at a
# time. For build.conf apps it iterates VERSIONS; others build once (version
# auto). Continues past failures and prints a summary. Honors PUSH / ARCHES.
#
# Usage: build-all.sh           # whole catalog, every version
#        build-all.sh <app>     # one app, every version
set -uo pipefail   # not -e: one failing build must not stop the rest
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"; cd "$ROOT"
export PATH="$HOME/.local/bin:$PATH"

FILTER="${1:-}"
if [ -n "$FILTER" ]; then
  [ -d "apps/$FILTER" ] || { echo "❌ no such app: apps/$FILTER"; exit 1; }
  DIRS=("apps/$FILTER/")
else
  DIRS=(apps/*/)
fi

ok=0; skip=0; fail=0; failed=""
for d in "${DIRS[@]}"; do
  a="$(basename "$d")"
  vers=""
  [ -f "$d/build.conf" ] && vers="$(bash -c "source '$d/build.conf'; printf '%s ' \"\${VERSIONS[@]:-}\"" 2>/dev/null || true)"
  [ -z "${vers// /}" ] && vers="__single__"
  for v in $vers; do
    [ "$v" = "__single__" ] && v=""
    label="$a${v:+:$v}"
    printf '\n════ %s ════\n' "$label"
    if PUSH="${PUSH:-1}" ARCHES="${ARCHES:-x86_64,aarch64}" scripts/build-image.sh "$a" "$v"; then
      ok=$((ok+1)); echo "✅ $label"
    elif [ $? -eq 2 ]; then
      skip=$((skip+1)); echo "⏭  $label (needs render config — skipped)"
    else
      fail=$((fail+1)); failed="$failed $label"; echo "❌ $label"
    fi
  done
done
echo ""
echo "════════════════════════════════════════════"
echo "build-all done: $ok built, $skip skipped, $fail failed.${failed:+"  Failed:$failed"}"
[ "$fail" -eq 0 ]
