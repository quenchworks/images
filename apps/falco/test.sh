#!/usr/bin/env bash
# Smoke test for a built Falco image. Usage: test.sh <image-ref> [version]
#
# No kernel capture in CI: it checks what runs without privileges.
#   1. falco --version reports the release.
#   2. falco --list-plugins loads the container plugin (config.d enables it).
#   3. falco -L loads the default rules, which require the container plugin.
#   4. falco -V validates the shipped rules file.
#   5. falco --list shows the plugin's container.* fields next to syscall ones.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
run() { docker run --rm "$IMAGE" "$@" 2>&1; }
need() { # need <label> <output> <fixed string>
  grep -qF -- "$3" <<<"$2" || { echo "$1: missing '$3'"; printf '%s\n' "$2" | tail -40; exit 1; }
}

out="$(run --version || true)"
echo "$out" | head -5
need "--version" "$out" "Falco version:"
if [ -n "${2:-}" ]; then need "--version" "$out" "Falco version: $2"; fi

out="$(run --list-plugins || true)"
echo "$out" | head -20
need "--list-plugins" "$out" "Name: container"
need "--list-plugins" "$out" "Version: 0.7.5"

out="$(run -L || true)"
echo "$out" | head -5
need "-L" "$out" "Terminal shell in container"

out="$(run -V /etc/falco/falco_rules.yaml || true)"
echo "$out" | tail -5
need "-V" "$out" ": Ok"

out="$(run --list || true)"
need "--list" "$out" "container.id"
need "--list" "$out" "proc.name"
echo "smoke test passed (version, container plugin, default rules load and validate, fields)"
