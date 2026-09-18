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
def _gh(path, jq):
    """One `gh api` call, RAISING when the call itself fails.

    Without this a rate-limited or network-failed call returns an empty stdout,
    which is indistinguishable from "this repo cuts no stable releases" and is
    reported as zero candidates. openbao read as BROKEN CHECK that way on
    2026-09-18 while upstream was in fact current at v2.6.2. An exception makes
    the sweep say the check ERRORED, which is the truth.
    """
    r = subprocess.run(["gh", "api", path, "-q", jq], capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"gh api {path} failed ({r.returncode}): {r.stderr.strip()[:300]}")
    return [t.strip().lstrip("v") for t in r.stdout.split() if t.strip()]

def github(repo):
    """Stable, non-draft release tags (v-stripped). Newest first-ish."""
    return _gh(f"repos/{repo}/releases?per_page=40",
               '.[]|select(.prerelease==false and .draft==false)|.tag_name')

def github_tags(repo):
    """Plain git tags (for repos that don't cut GitHub Releases)."""
    return _gh(f"repos/{repo}/tags?per_page=40", '.[].name')

def github_asset_releases(repo):
    """Release tags that actually ship a downloadable asset (prereleases included).

    Use this instead of github_tags() when the recipe fetches a RELEASE ASSET. Some
    projects cut preview/rc git TAGS without publishing a release (or publish a release
    with no binaries), so a tag-based check proposes a version whose artifact 404s --
    that is exactly how rustfs 1.0.0-beta.12-preview.1 reached VERSIONS and failed the
    build with curl exit 22. Filtering on assets keeps the checker honest about what is
    actually installable.
    """
    return _gh(f"repos/{repo}/releases?per_page=40",
               '.[]|select(.draft==false and (.assets|length)>0)|.tag_name')

def npm(pkg):
    data = json.load(urllib.request.urlopen(f"https://registry.npmjs.org/{pkg}", timeout=30))
    return list(data["versions"])

def pypi(pkg):
    data = json.load(urllib.request.urlopen(f"https://pypi.org/pypi/{pkg}/json", timeout=30))
    return list(data["releases"])

def scrape(url, pattern):
    """Fetch a directory-listing / page and return all regex group-1 matches."""
    html = urllib.request.urlopen(url, timeout=30).read().decode("utf-8", "replace")
    return re.findall(pattern, html)

def json_get(url):
    return json.load(urllib.request.urlopen(url, timeout=30))

def report_lines(app, candidates, depth, keep=None, clean=None):
    """Source-agnostic per-line check: from a flat candidate list, take the
    newest per version line (keyed to `depth` numeric components) and compare to
    our current entry on that line. Mirrors report_wolfi_lines for non-Wolfi
    sources (ftp listings, vendor JSON, git tags)."""
    cur = current(app)
    wmap = {}
    for c in candidates:
        if keep and not keep(c):
            continue
        cv = clean(c) if clean else c
        if not re.match(r'\d', cv):
            continue
        lk = ".".join(re.findall(r'\d+', cv)[:depth])
        if lk not in wmap or vkey(cv) > vkey(wmap[lk]):
            wmap[lk] = cv
    behind = []
    print(app)
    for c in cur:
        lk = ".".join(re.findall(r'\d+', c)[:depth])
        w = wmap.get(lk)
        upd = bool(w) and vkey(w) > vkey(c)
        if upd: behind.append(w)
        print(f"  {lk}: have={c:>12s}  latest={(w or '?'):>12s}  {'UPDATE' if upd else 'ok'}")
    ours_top = max((vkey(".".join(re.findall(r'\d+', c)[:depth])) for c in cur), default=(0,))
    newer = sorted((k for k in wmap if vkey(k) > ours_top), key=vkey, reverse=True)
    # EVERY new line, not just the newest. sealed-secrets sat behind both 0.39
    # and 0.40 while this printed only 0.40, so the app read as one line behind
    # when it was two and the older line looked like a deliberate skip.
    for k in newer:
        print(f"  NEW LINE available: {k} ({wmap[k]})")
    print(f"  => {'UPDATE -> ' + str(behind) if behind else ('NEW LINE ' + ' '.join(newer) if newer else 'ok')}")
    return behind

def wolfi(pkg):
    """Versions of a flat Wolfi apk (X.Y.Z, -rN stripped)."""
    out = subprocess.run(["python3", f"{BASE}/scripts/wolfi-latest.py", pkg, "40"],
                         capture_output=True, text=True).stdout.split()
    return [v.split("-r")[0] for v in out]

def wolfi_match(pattern):
    """{pkgname: best_version} for every Wolfi apk whose name matches `pattern`."""
    out = subprocess.run(["python3", f"{BASE}/scripts/wolfi-latest.py", "--match", pattern],
                         capture_output=True, text=True).stdout.splitlines()
    return {ln.split("\t")[0]: ln.split("\t")[1].split("-r")[0] for ln in out if "\t" in ln}

def report_wolfi_suffix(app, regex, label_fmt):
    """Per-line check for apks named with a suffix (e.g. dotnet-8-sdk,
    aspnet-9-runtime). `regex` matches them; the FIRST integer in the pkg name is
    the major/line. Compare each line's newest to our current entry sharing it."""
    m = wolfi_match(regex)
    byline = {}
    for p, v in m.items():
        maj = re.findall(r'\d+', p)[0]
        if maj not in byline or vkey(v) > vkey(byline[maj]):
            byline[maj] = v
    cur = current(app); behind = []
    print(app)
    for c in cur:
        maj = re.findall(r'\d+', c)[0]
        w = byline.get(maj)
        upd = bool(w) and vkey(w) > vkey(c)
        if upd: behind.append(w)
        print(f"  {label_fmt.format(maj=maj)}: have={c:>10s}  latest={(w or '?'):>10s}  {'UPDATE' if upd else 'ok'}")
    print(f"  => {'UPDATE -> ' + str(behind) if behind else 'ok'}")
    return behind

def wolfi_one(pkg):
    """Newest X.Y.Z of a single Wolfi apk, or None."""
    vs = wolfi(pkg)
    return sorted(vs, key=vkey, reverse=True)[0] if vs else None

def report_wolfi_lines(app, prefix, depth, line_only=False):
    """Per-line Wolfi check (like node): for each version line we ship (keyed to
    `depth` numeric components), compare Wolfi's newest patch for prefix-<line>
    against our current entry. prefix+line must be the real apk name (go-1.26,
    openjdk-21, php-8.4, ...).
    line_only: we pin the LINE only (e.g. php '8.3', patch floats) — compare at
    line granularity so a floating patch isn't a false 'UPDATE'; only a NEWER
    line counts."""
    cur = current(app)
    majors = wolfi_majors(prefix, 12)
    wmap = {}
    for w in majors:
        wmap[".".join(re.findall(r'\d+', w)[:depth])] = w
    behind = []
    print(app)
    for c in cur:
        lk = ".".join(re.findall(r'\d+', c)[:depth])
        w = wmap.get(lk)
        cmpw = vkey(".".join(re.findall(r'\d+', w)[:depth])) if (w and line_only) else (vkey(w) if w else None)
        upd = bool(w) and cmpw > vkey(c)
        if upd: behind.append(w)
        print(f"  {prefix}{lk}: have={c:>12s}  latest={(w or '?'):>12s}  {'UPDATE' if upd else 'ok'}")
    # a brand-new upstream line beyond our newest (window-shift candidate)
    ours_top = max((vkey(".".join(re.findall(r'\d+', c)[:depth])) for c in cur), default=(0,))
    newer = sorted((k for k in wmap if vkey(k) > ours_top), key=vkey, reverse=True)
    if newer:
        for k in newer:
            print(f"  NEW LINE available: {prefix}{k} ({wmap[k]})")
    print(f"  => {'UPDATE -> ' + str(behind) if behind else ('NEW LINE ' + ' '.join(newer) if newer else 'ok')}")
    return behind

def wolfi_majors(prefix, n=4):
    """Newest patch of the latest N major lines for versioned Wolfi apks
    (e.g. prefix 'go-' -> go-1.26 / go-1.25 ...). Returns version strings."""
    out = subprocess.run(["python3", f"{BASE}/scripts/wolfi-latest.py", "--majors", prefix, str(n)],
                         capture_output=True, text=True).stdout.split("\n")
    return [ln.split("->")[-1].strip().split("-r")[0] for ln in out if "->" in ln]

# ---- report --------------------------------------------------------------
def report(app, candidates, n=None, keep=None, clean=None):
    """Compare the newest `n` source versions to what we ship.
    keep:  predicate to filter raw candidates (e.g. drop prereleases/majors).
    clean: map a raw tag to a version string (e.g. 'REL_17_2' -> '17.2')."""
    cur = current(app)
    n = n or len(cur) or 3
    cands = []
    for c in candidates:
        if keep and not keep(c):
            continue
        cv = clean(c) if clean else c
        if re.match(r'\d', cv):
            cands.append(cv)
    top = sorted(set(cands), key=vkey, reverse=True)[:n]
    have = vkey(cur[-1]) if cur else (0,)
    behind = [v for v in top if vkey(v) > have]

    # An EMPTY upstream result is a BROKEN CHECK, not "up to date".
    #
    # This used to print "ok" and be counted in `0 errors`, which is how mongodb sat at
    # latest3=[] indefinitely: its checker returned nothing (upstream moved off the tag
    # pattern it scrapes) and every run reported the app as fine. Same failure shape as
    # gen-catalog treating an api blip as "app unpublished" -- silence read as success.
    #
    # Also flag have > newest-upstream: that means the checker is looking at the WRONG
    # SOURCE (yarn's checker reads yarn 1.x classic tags while we ship berry 4.x, and
    # coolify-app reports 4.1.2 while we ship 4.2.0). Both would hide a real update
    # forever while printing ok.
    if not top:
        status = "BROKEN CHECK -- upstream returned NO candidates (not 'up to date')"
    elif vkey(top[0]) < have:
        status = f"BROKEN CHECK -- we ship {cur[-1]} but upstream's newest is {top[0]}; wrong source?"
    else:
        status = f"UPDATE -> {behind}" if behind else "ok"
    print(f"{app:20s} have={(cur[-1] if cur else '?'):>14s}  latest{n}={top}  {status}")
    return behind

if __name__ == "__main__":
    print("this is the shared lib; run scripts/check/<app>.py", file=sys.stderr)
