#!/usr/bin/env python3
"""Latest Wolfi apk versions for a package — the single source for every
FROM_SOURCE=0 (apko-only) app, whose versions come from the Wolfi apk repo,
NOT from upstream (nodejs.org / npm / GitHub). pnpm, node, uv all live here.

Usage:
  wolfi-latest.py <pkg> [N]            # latest N distinct X.Y.Z + best -rN
  wolfi-latest.py --majors <prefix> N  # newest patch of the latest N major lines
                                       #   (e.g. --majors nodejs- 4)

Examples:
  wolfi-latest.py uv 3
  wolfi-latest.py pnpm-11 1
  wolfi-latest.py --majors nodejs- 4
"""
import sys, re, io, tarfile, urllib.request

IDX = "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz"

def vkey(v):  # "10.33.0-r1" -> (10,33,0,1) for correct numeric ordering
    nums = re.findall(r'\d+', v)
    return tuple(int(n) for n in nums)

def load():
    raw = urllib.request.urlopen(IDX, timeout=60).read()
    with tarfile.open(fileobj=io.BytesIO(raw)) as t:
        body = t.extractfile("APKINDEX").read().decode()
    # APKINDEX is blank-line-delimited records with P:/V: lines
    out = {}
    for rec in body.split("\n\n"):
        p = v = None
        for ln in rec.splitlines():
            if ln.startswith("P:"): p = ln[2:]
            elif ln.startswith("V:"): v = ln[2:]
        if p and v:
            out.setdefault(p, []).append(v)
    return out

def best_per_release(versions):
    """Collapse to the highest -rN per X.Y.Z, return list sorted desc."""
    by_xyz = {}
    for v in versions:
        xyz = v.split("-r")[0]
        if xyz not in by_xyz or vkey(v) > vkey(by_xyz[xyz]):
            by_xyz[xyz] = v
    return sorted(by_xyz.values(), key=vkey, reverse=True)

def main():
    a = sys.argv[1:]
    idx = load()
    if a and a[0] == "--majors":
        prefix, n = a[1], int(a[2]) if len(a) > 2 else 4
        # packages named <prefix><major> with a purely-numeric major suffix
        # major suffix may be an int (erlang-27, nodejs-26) or dotted (go-1.26)
        majors = {}
        for p in idx:
            m = re.fullmatch(re.escape(prefix) + r'(\d+(?:\.\d+)*)', p)
            if m:
                majors[tuple(int(x) for x in m.group(1).split("."))] = p
        for maj in sorted(majors, reverse=True)[:n]:
            newest = best_per_release(idx[majors[maj]])[0]
            print(f"{majors[maj]}  ->  {newest}")
        return
    pkg = a[0]; n = int(a[1]) if len(a) > 1 else 3
    if pkg not in idx:
        print(f"NOT IN WOLFI: {pkg}", file=sys.stderr); sys.exit(1)
    for v in best_per_release(idx[pkg])[:n]:
        print(v)

if __name__ == "__main__":
    main()
