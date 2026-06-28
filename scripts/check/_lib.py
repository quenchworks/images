#!/usr/bin/env python3
"""Shared plumbing for the per-app upstream checks in scripts/check/<app>.py.

Each <app>.py is tiny: it knows ONLY that app's source + version quirks, fetches
the candidate versions, and calls report(). All HTTP / sorting / "do we already
have it?" logic lives here so the per-app files stay a few lines each.

Run one:   python3 scripts/check/opa.py
Run all:   for f in scripts/check/*.py; do [ "$f" = scripts/check/_lib.py ] || python3 "$f"; done
"""
import subprocess, re, json, os, sys, urllib.request

BASE = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))  # images/

def vkey(v):
    return tuple(int(n) for n in re.findall(r'\d+', v))

def current(app):
    """The app's current VERSIONS array (the window) from its build.conf."""
    out = subprocess.run(
        ["bash", "-c", f"source {BASE}/apps/{app}/build.conf; echo ${{VERSIONS[@]}}"],
        capture_output=True, text=True).stdout.split()
    return out

# ---- sources -------------------------------------------------------------
def github(repo):
    """Stable, non-draft release tags (v-stripped). Newest first-ish."""
    out = subprocess.run(
        ["gh", "api", f"repos/{repo}/releases?per_page=40", "-q",
         '.[]|select(.prerelease==false and .draft==false)|.tag_name'],
        capture_output=True, text=True).stdout.split()
    return [t.strip().lstrip("v") for t in out if t.strip()]

def github_tags(repo):
    """Plain git tags (for repos that don't cut GitHub Releases)."""
    out = subprocess.run(
        ["gh", "api", f"repos/{repo}/tags?per_page=40", "-q", '.[].name'],
        capture_output=True, text=True).stdout.split()
    return [t.strip().lstrip("v") for t in out if t.strip()]

def npm(pkg):
    data = json.load(urllib.request.urlopen(f"https://registry.npmjs.org/{pkg}", timeout=30))
    return list(data["versions"])

def pypi(pkg):
    data = json.load(urllib.request.urlopen(f"https://pypi.org/pypi/{pkg}/json", timeout=30))
    return list(data["releases"])

def wolfi(pkg):
    """Versions of a flat Wolfi apk (X.Y.Z, -rN stripped)."""
    out = subprocess.run(["python3", f"{BASE}/scripts/wolfi-latest.py", pkg, "40"],
                         capture_output=True, text=True).stdout.split()
    return [v.split("-r")[0] for v in out]

def wolfi_majors(prefix, n=4):
    """Newest patch of the latest N major lines for versioned Wolfi apks
    (e.g. prefix 'go-' -> go-1.26 / go-1.25 ...). Returns version strings."""
    out = subprocess.run(["python3", f"{BASE}/scripts/wolfi-latest.py", "--majors", prefix, str(n)],
                         capture_output=True, text=True).stdout.split("\n")
    return [ln.split("->")[-1].strip().split("-r")[0] for ln in out if "->" in ln]

# ---- report --------------------------------------------------------------
def report(app, candidates, n=None, keep=None):
    """Compare the newest `n` source versions to what we ship.
    keep: optional predicate to filter candidate versions (e.g. drop majors)."""
    cur = current(app)
    n = n or len(cur) or 3
    cands = [c for c in candidates if (keep(c) if keep else True) and re.match(r'\d', c)]
    top = sorted(set(cands), key=vkey, reverse=True)[:n]
    have = vkey(cur[-1]) if cur else (0,)
    behind = [v for v in top if vkey(v) > have]
    status = f"UPDATE -> {behind}" if behind else "ok"
    print(f"{app:20s} have={(cur[-1] if cur else '?'):>14s}  latest{n}={top}  {status}")
    return behind

if __name__ == "__main__":
    print("this is the shared lib; run scripts/check/<app>.py", file=sys.stderr)
