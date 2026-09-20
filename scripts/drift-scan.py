#!/usr/bin/env python3
"""Scan PUBLISHED images for CVEs that appeared after they were built.

    uv run --with pyyaml scripts/drift-scan.py [--shard N --of M] [--json out.json]

The build gate proves an image was clean the day it was built. Nothing re-checks
it afterwards, so a new advisory lands on a shipped image with no signal at all:
on 2026-09-20 that had happened to 34 images, found only because someone thought
to look. This is the check that would have found them.

It scans the NEWEST published version of each app, which is what people pull.
Older tags drift too; scanning every one costs several hours of runner time for
tags most users never pull, so this deliberately covers the newest only. Widen
it when a stale-tag finding actually bites.

Exit 1 when any image has findings, so a scheduled run fails visibly.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
TRIVY = ["trivy", "image", "--ignore-unfixed", "--severity", "CRITICAL,HIGH,MEDIUM,LOW",
         "--scanners", "vuln", "--detection-priority", "comprehensive", "--quiet", "-f", "json"]


def newest(lock) -> list[tuple[str, str, str]]:
    out = []
    for name, app in sorted(lock["apps"].items()):
        vs = app.get("versions") or []
        if not vs:
            continue
        v = max(vs, key=lambda x: x["published"])
        out.append((name, v["version"], app["image"]))
    return out


def scan(ref: str) -> list[dict] | None:
    """Findings for one image, or None if the scan itself failed.

    None matters: a Trivy failure is not a clean image. Running these in
    parallel produced exactly that confusion, so they run one at a time.
    """
    try:
        p = subprocess.run(TRIVY + [ref], capture_output=True, text=True, timeout=600)
        if p.returncode not in (0, 1) or not p.stdout.strip():
            return None
        d = json.loads(p.stdout)
    except Exception:
        return None
    return [
        {"pkg": v.get("PkgName"), "id": v.get("VulnerabilityID"),
         "severity": v.get("Severity"), "installed": v.get("InstalledVersion"),
         "fixed": v.get("FixedVersion")}
        for r in (d.get("Results") or [])
        for v in (r.get("Vulnerabilities") or [])
    ]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--shard", type=int, default=0)
    ap.add_argument("--of", type=int, default=1)
    ap.add_argument("--json", type=pathlib.Path)
    ap.add_argument("--only", nargs="*", help="scan just these apps")
    args = ap.parse_args()

    lock = yaml.safe_load((ROOT / "catalog.lock.yaml").read_text())
    targets = newest(lock)
    if args.only:
        want = set(args.only)
        targets = [t for t in targets if t[0] in want]
    else:
        targets = [t for i, t in enumerate(targets) if i % args.of == args.shard]

    dirty, failed = [], []
    for name, ver, image in targets:
        ref = "%s:%s" % (image, ver)
        found = scan(ref)
        if found is None:
            failed.append(name)
            print("%-28s SCAN FAILED" % name, flush=True)
        elif found:
            dirty.append({"app": name, "version": ver, "findings": found})
            worst = ",".join(sorted({f["severity"] or "?" for f in found}))
            print("%-28s %d finding(s) [%s]" % (name, len(found), worst), flush=True)
        else:
            print("%-28s clean" % name, flush=True)

    print("\nscanned %d | dirty %d | scan-failed %d" % (len(targets), len(dirty), len(failed)))
    if args.json:
        args.json.write_text(json.dumps({"dirty": dirty, "failed": failed}, indent=1))
    return 1 if (dirty or failed) else 0


if __name__ == "__main__":
    raise SystemExit(main())
