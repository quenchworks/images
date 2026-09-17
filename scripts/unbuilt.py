#!/usr/bin/env python3
"""Find recipes whose newest VERSIONS entry was never actually built.

A committed version bump is not a published image, and nothing connects the two:
build workflows are workflow_dispatch only, so a recipe can sit ahead of the
registry indefinitely with every run green and every file correct.

Usage:
  unbuilt.py            # sweep every unblocked app
  unbuilt.py --all      # include BLOCKED apps (expected to be behind; noisy)
  unbuilt.py zot linkerd-proxy

Found on 2026-09-17, the first time it ran: zot 2.1.21 committed 4 days earlier
and never dispatched, and linkerd-proxy, linkerd-control-plane and
linkerd-policy-controller, whose recipes had been bumped 18 days earlier and
never built once. That last case hid eight separate build failures behind one
another and blocked a chart that could not release without them.

BLOCKED apps are skipped by default and are not a finding: prep deliberately
skips build and merge for them, so their newest version is expected to be
absent. `--all` includes them when you want the full picture.
"""
import argparse
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
REF = "ghcr.io/quenchworks/images/{app}:{ver}"


def newest_version(app: str) -> str | None:
    """The last VERSIONS entry, read by SOURCING build.conf under bash.

    Parsing it with a regex breaks on every recipe that computes VERSIONS, and
    zsh cannot be used here: it does not word-split unquoted variables.
    """
    r = subprocess.run(
        ["bash", "-c", f'cd "{ROOT}/apps/{app}" && source build.conf && echo "${{VERSIONS[-1]}}"'],
        capture_output=True, text=True,
    )
    v = r.stdout.strip()
    return v or None


def published(app: str, ver: str) -> bool:
    r = subprocess.run(
        ["docker", "buildx", "imagetools", "inspect", REF.format(app=app, ver=ver),
         "--format", "{{.Manifest.Digest}}"],
        capture_output=True, text=True,
    )
    return r.returncode == 0 and r.stdout.strip().startswith("sha256:")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("apps", nargs="*", help="apps to check (default: all)")
    ap.add_argument("--all", action="store_true", help="include BLOCKED apps")
    args = ap.parse_args()

    apps = args.apps or sorted(p.name for p in (ROOT / "apps").iterdir() if (p / "build.conf").exists())
    unbuilt, skipped = [], 0
    for app in apps:
        conf = ROOT / "apps" / app / "build.conf"
        if not conf.exists():
            print(f"!! no such app: {app}", file=sys.stderr)
            continue
        blocked = any(l.startswith("BLOCKED=1") for l in conf.read_text().splitlines())
        if blocked and not args.all:
            skipped += 1
            continue
        ver = newest_version(app)
        if not ver:
            print(f"!! {app}: could not read VERSIONS", file=sys.stderr)
            continue
        if not published(app, ver):
            unbuilt.append((app, ver, blocked))
            print(f"UNBUILT {app} {ver}" + ("  (BLOCKED, expected)" if blocked else ""))

    real = [u for u in unbuilt if not u[2]]
    print(f"\nchecked {len(apps) - skipped} apps, skipped {skipped} blocked, "
          f"{len(real)} recipe(s) ahead of the registry")
    if real:
        print("dispatch them: " + " ".join(f"gh workflow run build-{a}.yml -R quenchworks/images" for a, _, _ in real))
    return 1 if real else 0


if __name__ == "__main__":
    raise SystemExit(main())
