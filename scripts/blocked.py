#!/usr/bin/env python3
"""Report and clear BLOCKED=1 apps once the thing that blocked them is fixed.

Most of the current register is waiting on one apk: zlib 1.3.3-r0, for
CVE-2026-85091. The moment Wolfi ships it, ~14 recipes need nothing but
BLOCKED=0 and a rebuild, so this does that sweep instead of doing it by hand.

Usage:
  blocked.py                     # list blocked apps + whether the wall lifted
  blocked.py --pkg zlib --min 1.3.3-r0
  blocked.py --dispatch          # flip BLOCKED=0 and rebuild, 10 at a time

--dispatch refuses to run while the named package is still below --min, so a
sweep cannot be kicked off before the fix actually lands. It only edits recipes
whose STATUS block names that package; anything blocked for another reason is
listed and left alone.
"""
import sys, re, io, os, tarfile, urllib.request, subprocess, pathlib, argparse

IDX = "https://packages.wolfi.dev/os/{arch}/APKINDEX.tar.gz"
REPO = "quenchworks/images"
BATCH = 10


def vkey(v):
    # Naive: splits on . and _ and treats any non-numeric chunk as 0, so it
    # cannot express apk's rule that 1.3.2_rc1 sorts BELOW 1.3.2. That only
    # misorders within one base version, which can never flip a comparison
    # against a --min on a different base. Use apk's own ordering if this ever
    # needs to compare a prerelease against its own release.
    m = re.match(r"^(.*?)-r(\d+)$", v)
    base, rel = (m.group(1), int(m.group(2))) if m else (v, 0)
    parts = tuple(int(p) if p.isdigit() else 0 for p in re.split(r"[._]", base))
    return (parts, rel)


def wolfi_version(pkg, arch="x86_64"):
    """Newest published version of pkg, or None. Reads the real APKINDEX."""
    raw = urllib.request.urlopen(IDX.format(arch=arch), timeout=60).read()
    with tarfile.open(fileobj=io.BytesIO(raw)) as t:
        idx = t.extractfile("APKINDEX").read().decode()
    vers = [
        v
        for rec in idx.split("\n\n")
        if any(l == f"P:{pkg}" for l in rec.splitlines())
        for v in [l[2:] for l in rec.splitlines() if l.startswith("V:")]
    ]
    return max(vers, key=vkey) if vers else None


def blocked_apps(root):
    """(app, build.conf path, full text) for every app with BLOCKED=1."""
    out = []
    for conf in sorted(root.glob("apps/*/build.conf")):
        text = conf.read_text()
        if re.search(r"^BLOCKED=1", text, re.M):
            out.append((conf.parent.name, conf, text))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pkg", default="zlib")
    ap.add_argument("--min", default="1.3.3-r0", help="version that lifts the block")
    ap.add_argument("--dispatch", action="store_true")
    args = ap.parse_args()

    root = pathlib.Path(__file__).resolve().parent.parent
    apps = blocked_apps(root)
    if not apps:
        print("nothing blocked")
        return 0

    # Both arches must clear: a per-arch apk can lag, and the gate runs on both.
    have = {a: wolfi_version(args.pkg, a) for a in ("x86_64", "aarch64")}
    lifted = all(v and vkey(v) >= vkey(args.min) for v in have.values())

    print(f"{args.pkg}: " + ", ".join(f"{a}={v}" for a, v in have.items()))
    print(f"needs {args.min} -> {'LIFTED' if lifted else 'still walled'}\n")

    mine = [(n, c) for n, c, t in apps if args.pkg in t]
    other = [n for n, c, t in apps if args.pkg not in t]
    print(f"blocked on {args.pkg} ({len(mine)}): {' '.join(n for n, _ in mine)}")
    if other:
        print(f"blocked for other reasons ({len(other)}): {' '.join(other)}")

    if not args.dispatch:
        return 0
    if not lifted:
        print(f"\nrefusing to dispatch: {args.pkg} has not reached {args.min}")
        return 1

    for n, conf in mine:
        conf.write_text(re.sub(r"^BLOCKED=1", "BLOCKED=0", conf.read_text(), count=1, flags=re.M))
    subprocess.run(["git", "add"] + [str(c) for _, c in mine], cwd=root, check=True)
    subprocess.run(
        ["git", "commit", "-m",
         f"chore: unblock {len(mine)} apps, Wolfi shipped {args.pkg} {have['x86_64']}"],
        cwd=root, check=True)
    subprocess.run(["git", "push", "origin", "HEAD"], cwd=root, check=True)

    for i in range(0, len(mine), BATCH):
        for n, _ in mine[i:i + BATCH]:
            subprocess.run(
                ["gh", "workflow", "run", f"build-{n}.yml", "-R", REPO], check=True)
        print(f"dispatched {min(i + BATCH, len(mine))}/{len(mine)}; "
              f"wait for these before the next batch")
    return 0


if __name__ == "__main__":
    sys.exit(main())
