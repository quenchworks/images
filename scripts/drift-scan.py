#!/usr/bin/env python3
"""Scan PUBLISHED images for CVEs that appeared after they were built.

    uv run --with pyyaml scripts/drift-scan.py [--shard N --of M] [--json out.json]

The build gate proves an image was clean the day it was built. Nothing re-checks
it afterwards, so a new advisory lands on a shipped image with no signal at all:
on 2026-09-20 that had happened to 34 images, found only because someone thought
to look. This is the check that would have found them.

It scans EVERY published version in the lock, not just the newest.

It used to cover the newest only, on the argument that older tags cost runner
time for images most people never pull. A 12-tag sample of the older ones on
2026-09-20 came back 2 dirty, and one was grafana 13.0.2 with 52 findings
including CRITICAL. An old tag is still something a user can pull and the
catalog still claims is 0-CVE.

To be exact about what widening this bought, because the first version of this
note overstated it: the website's nightly (website/scripts/scan-images.mjs) had
already scanned every version AND every chart-pinned digest, so those tags were
not unwatched. What was missing is that THIS check, the one that opens the
tracking issue in the images repo, disagreed with it. Two scanners over the same
images have to ask the same question or the issue quietly under-reports. Same
shape as the VEX gap fixed the same day.

Blocked apps are where it bites hardest. grafana never rebuilds, so all six of
its tags keep drifting: 17, 43, 49, 52, 78 and 81 findings, newest to oldest.

Exit 1 when any image has findings, so a scheduled run fails visibly.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
TRIVY = ["trivy", "image", "--ignore-unfixed", "--severity", "CRITICAL,HIGH,MEDIUM,LOW",
         "--scanners", "vuln", "--detection-priority", "comprehensive", "--quiet", "-f", "json"]


def all_versions(lock) -> list[tuple[str, str, str]]:
    """Every published version of every app, newest first within each app.

    Newest first matters for sharding: a shard that stalls has still covered the
    tags most people pull before it ran out of time.
    """
    out = []
    for name, app in sorted(lock["apps"].items()):
        vs = sorted(app.get("versions") or [], key=lambda x: x["published"], reverse=True)
        for v in vs:
            out.append((name, v["version"], app["image"]))
    return out


def scan(app: str, ref: str) -> list[dict] | None:
    """Findings for one image, or None if the scan itself failed.

    None matters: a Trivy failure is not a clean image. Running these in
    parallel produced exactly that confusion, so they run one at a time.

    The build gate passes apps/<app>/vex.openvex.json to Trivy (build-image.sh),
    so an app whose clearance is documented there ships green. Scanning without
    it re-reports those same cleared advisories as drift, which is how grafana,
    jenkins-inbound-agent and kgateway landed on the first run's list. Apply the
    identical VEX here so the two scanners agree; anything not in the VEX file
    is still a finding.
    """
    vex = ROOT / "apps" / app / "vex.openvex.json"
    extra = ["--vex", str(vex)] if vex.is_file() else []
    try:
        p = subprocess.run(TRIVY + extra + [ref], capture_output=True, text=True, timeout=600)
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
    targets = all_versions(lock)
    if args.only:
        want = set(args.only)
        targets = [t for t in targets if t[0] in want]
    else:
        targets = [t for i, t in enumerate(targets) if i % args.of == args.shard]

    dirty, failed = [], []
    for name, ver, image in targets:
        ref = "%s:%s" % (image, ver)
        found = scan(name, ref)
        if found is None:
            # app AND version: one app can now fail on one tag and pass on another.
            failed.append("%s %s" % (name, ver))
            print("%-28s %-14s SCAN FAILED" % (name, ver), flush=True)
        elif found:
            dirty.append({"app": name, "version": ver, "findings": found})
            worst = ",".join(sorted({f["severity"] or "?" for f in found}))
            print("%-28s %-14s %d finding(s) [%s]" % (name, ver, len(found), worst), flush=True)
        else:
            print("%-28s %-14s clean" % (name, ver), flush=True)

    print("\nscanned %d | dirty %d | scan-failed %d" % (len(targets), len(dirty), len(failed)))
    if args.json:
        args.json.write_text(json.dumps({"dirty": dirty, "failed": failed}, indent=1))
    return 1 if (dirty or failed) else 0


if __name__ == "__main__":
    raise SystemExit(main())
