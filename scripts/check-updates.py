#!/usr/bin/env python3
"""Run the per-app version checkers in scripts/check/ and report have-vs-latest.

Each scripts/check/<app>.py hits that app's real upstream and prints one line:
    <app>  have=<shipped>  latestN=[...]  ok | UPDATE -> [<newer>...]

Usage:
    uv run scripts/check-updates.py               # every app, in parallel
    uv run scripts/check-updates.py airflow redis # just these apps
    uv run scripts/check-updates.py -q            # only apps with an update
    uv run scripts/check-updates.py -j 12         # set parallelism (default 8)

Exit code is the number of apps with an available update (0 = all current),
so CI / cron can gate on it.
"""
import sys
import subprocess
import concurrent.futures as cf
from pathlib import Path

CHECK_DIR = Path(__file__).resolve().parent / "check"


def run_one(py: Path) -> str:
    """Run a single checker (from the check/ dir so `from _lib import ...` works)."""
    try:
        r = subprocess.run(
            [sys.executable, py.name],
            cwd=CHECK_DIR,
            capture_output=True,
            text=True,
            timeout=90,
        )
        out = (r.stdout.strip() or r.stderr.strip())
        # A healthy checker line always contains "have="; anything else is a failure.
        return out if "have=" in out else f"{py.stem:20s} ERROR: {(out.splitlines() or ['no output'])[-1][:80]}"
    except subprocess.TimeoutExpired:
        return f"{py.stem:20s} ERROR: timed out"


def apps_without_checker(checks):
    """Apps in apps/ with no checker file.

    An app with no checker is INVISIBLE to this tool: it is never reported as
    behind, at any age. That is not a gap in coverage, it is a gap that LOOKS
    like coverage, because the summary line only counts apps it happened to
    check. vikunja sat two releases behind carrying 2 HIGH + 3 MEDIUM CVEs
    without ever appearing as an UPDATE for exactly this reason.
    """
    apps_dir = CHECK_DIR.parent.parent / "apps"
    if not apps_dir.is_dir():
        return []
    have = {c.stem for c in checks}
    return sorted(
        d.name for d in apps_dir.iterdir()
        if (d / "build.conf").exists() and d.name not in have
    )


def main() -> int:
    argv = sys.argv[1:]
    quiet = False
    workers = 8
    names: list[str] = []
    it = iter(argv)
    for a in it:
        if a in ("-q", "--updates-only"):
            quiet = True
        elif a in ("-j", "--jobs"):
            workers = int(next(it))
        elif a in ("-h", "--help"):
            print(__doc__)
            return 0
        elif a.startswith("-"):
            sys.exit(f"unknown flag: {a}")
        else:
            names.append(a)

    uncovered = []
    if names:
        checks = [CHECK_DIR / f"{n}.py" for n in names]
        missing = [c.stem for c in checks if not c.exists()]
        if missing:
            sys.exit(f"no checker for: {', '.join(missing)}")
    else:
        checks = sorted(p for p in CHECK_DIR.glob("*.py") if p.name != "_lib.py")
        uncovered = apps_without_checker(checks)

    with cf.ThreadPoolExecutor(max_workers=workers) as ex:
        lines = list(ex.map(run_one, checks))

    updates = sorted(l for l in lines if "UPDATE ->" in l)
    errors = sorted(l for l in lines if "have=" not in l)
    current = sorted(l for l in lines if "have=" in l and "UPDATE ->" not in l)

    if quiet:
        print("\n".join(updates) if updates else "all current")
    else:
        for l in current + updates:
            print(l)
        if errors:
            print("\n-- errors --")
            print("\n".join(errors))

    # A checker that returns no candidates is indistinguishable from "current" in
    # the summary, so name those explicitly -- same silent-failure shape as a
    # missing checker.
    blank = sorted(l for l in lines if "have=" in l and "latest" in l and "=[]" in l)
    if blank:
        print("\n-- checkers returning ZERO candidates (treat as BROKEN, not current) --")
        print("\n".join(blank))

    if uncovered:
        print(f"\n-- {len(uncovered)} apps with NO checker (never reported as behind) --")
        for i in range(0, len(uncovered), 8):
            print("   " + " ".join(uncovered[i:i + 8]))
        print("   each needs scripts/check/<slug>.py -- see docs/update-plan-2026-08-31.md")

    total = len(checks) + len(uncovered)
    print(
        f"\nsummary: {len(checks)}/{total} apps checked · {len(updates)} updates"
        f" · {len(errors)} errors · {len(blank)} blank · {len(uncovered)} unchecked"
    )
    return len(updates)


if __name__ == "__main__":
    sys.exit(main())
