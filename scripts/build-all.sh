#!/usr/bin/env bash
# Build EVERY app, EVERY version, one at a time (the full local cutover).
# For build.conf apps it iterates VERSIONS; others build once (version auto).
# Continues past failures and prints a summary. Honors PUSH / ARCHES from env.
set -uo pipefail   # not -e: one failing app must not stop the catalog
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"; cd "$ROOT"
export PATH="$HOME/.local/bin:$PATH"

ok=0; skip=0; fail=0; failed=""
for d in apps/*/; do
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
