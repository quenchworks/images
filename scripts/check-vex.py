#!/usr/bin/env -S uv run --quiet
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyyaml>=6"]
# ///
"""Validate every apps/*/vex.openvex.json against the house rules.

    uv run scripts/check-vex.py [app ...]        # default: every app that has one

WHY THIS EXISTS. build-image.sh passes `--vex apps/<app>/vex.openvex.json` to Trivy
when that file is present, which means the file can REMOVE findings from the 0-CVE
gate. That gate is the whole product guarantee, so a VEX file is not documentation:
it is production suppression logic, and an over-eager entry hides a real CVE from
the one check meant to catch it. Nothing enforced its contents until this script.

THE RULES, and the reasoning behind each:

  status must be `not_affected`
      VEX can also say `affected`, `fixed` or `under_investigation`. None of those
      belong here. `under_investigation` in particular would let an unproven hunch
      suppress a finding, which is the exact failure mode this file guards.

  justification must be one of the five OpenVEX values
      A free-text reason is not auditable. The enum forces the author to say which
      KIND of not-affected claim they are making.

  impact_statement must be substantive and cite how it was verified
      "false positive" is an assertion. The gate deserves evidence: what was
      checked, with what command, and what the result was. Enforced as a length
      floor plus at least one evidence marker, which is a heuristic -- it cannot
      tell truth from fiction, only laziness from effort.

  every CVE appears once per product
      Duplicate statements for the same (CVE, product) mean two authors disagreed
      or one edit was half-applied.

This script cannot verify a claim is TRUE. A reviewer still has to read the
impact_statement and agree. It only guarantees the claim is well-formed, narrow,
and accompanied by an argument.
"""
from __future__ import annotations

import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

# https://openvex.dev/ns/v0.2.0 -- the complete set. No others are valid.
JUSTIFICATIONS = {
    "component_not_present",
    "vulnerable_code_not_present",
    "vulnerable_code_not_in_execute_path",
    "vulnerable_code_cannot_be_controlled_by_adversary",
    "inline_mitigations_already_exist",
}

MIN_IMPACT = 120
# Words that indicate the author actually looked rather than assumed. Deliberately
# broad: the point is to reject "not applicable" one-liners, not to police phrasing.
EVIDENCE_MARKERS = (
    "verified", "verify", "confirmed", "extract", "grep", "0 matches", "no matches",
    "inspected", "checked", "reproduce", "does not ship", "not present", "absent",
    "measured", "read the source", "disassembl", "strings ", "sbom",
)


def check_file(path: pathlib.Path) -> list[str]:
    errs: list[str] = []
    try:
        doc = json.loads(path.read_text())
    except json.JSONDecodeError as e:
        return [f"invalid JSON: {e}"]

    ctx = str(doc.get("@context", ""))
    if "openvex.dev/ns" not in ctx:
        errs.append(f"@context is not an OpenVEX namespace: {ctx!r}")
    for field in ("@id", "author", "timestamp", "statements"):
        if not doc.get(field):
            errs.append(f"missing required top-level field: {field}")

    seen: set[tuple[str, str]] = set()
    for i, st in enumerate(doc.get("statements") or []):
        where = f"statement[{i}]"
        cve = ((st.get("vulnerability") or {}).get("name") or "").strip()
        if not cve:
            errs.append(f"{where}: no vulnerability.name")
        elif not cve.startswith(("CVE-", "GHSA-", "GO-")):
            errs.append(f"{where}: {cve!r} is not a CVE/GHSA/GO identifier")

        products = st.get("products") or []
        if not products:
            errs.append(f"{where} ({cve}): no products -- a statement must name what it applies to")
        for p in products:
            pid = (p or {}).get("@id", "")
            if not pid:
                errs.append(f"{where} ({cve}): a product has no @id")
                continue
            key = (cve, pid)
            if key in seen:
                errs.append(f"{where}: duplicate statement for {cve} on {pid}")
            seen.add(key)

        status = st.get("status")
        if status != "not_affected":
            errs.append(
                f"{where} ({cve}): status is {status!r}; only 'not_affected' may appear "
                f"in a suppression file"
            )

        just = st.get("justification")
        if just not in JUSTIFICATIONS:
            errs.append(f"{where} ({cve}): justification {just!r} is not an OpenVEX value")

        impact = (st.get("impact_statement") or "").strip()
        if len(impact) < MIN_IMPACT:
            errs.append(
                f"{where} ({cve}): impact_statement is {len(impact)} chars; needs >= {MIN_IMPACT} "
                f"explaining what was checked and what it showed"
            )
        elif not any(m in impact.lower() for m in EVIDENCE_MARKERS):
            errs.append(
                f"{where} ({cve}): impact_statement cites no verification -- say how the claim "
                f"was checked, not just that it is a false positive"
            )
    return errs


def check_workflow_wiring(app: str) -> list[str]:
    """A VEX file only reaches the CI gate if its workflow passes TRIVY_VEX.

    The local gate (build-image.sh) picks the file up automatically, so a missing CI
    wiring is INVISIBLE until a publish attempt fails -- which happened three separate
    times on 2026-08-27 (grafana, jenkins-inbound-agent, kgateway), each because a
    workflow was copied from an app that had no VEX. trivy-action has no `vex:` input,
    so the file has to go through the TRIVY_VEX env var; and the validator itself must
    run there too, or CI would honour an unchecked suppression file.
    """
    wf = ROOT / ".github" / "workflows" / f"build-{app}.yml"
    if not wf.exists():
        return [f"no workflow at {wf.relative_to(ROOT)} to check"]
    text = wf.read_text()
    errs = []
    if f"TRIVY_VEX: apps/{app}/vex.openvex.json" not in text:
        errs.append(
            f"build-{app}.yml does not pass TRIVY_VEX -- the VEX clears the LOCAL gate "
            f"but CI will still fail on the suppressed findings"
        )
    if "check-vex.py" not in text:
        errs.append(
            f"build-{app}.yml does not run check-vex.py -- CI would honour this file "
            f"without validating it"
        )
    return errs


def check_stale(app: str, path: pathlib.Path) -> list[str]:
    """Report statements whose CVE no longer appears in the published image.

    docs/vex-policy.md: "prune entries whose CVE no longer appears in a scan of
    the current image", because a stale statement can suppress a future finding
    that is real. 16 of jenkins-inbound-agent's 25 statements were stale when
    this was first checked by hand on 2026-09-17, so the drift is fast enough to
    be worth a command.

    Scans WITHOUT --vex, every severity, no --ignore-unfixed, so a CVE that is
    merely filtered out is never mistaken for one that is gone. Reports why it
    could not run rather than returning clean: a staleness check that silently
    passes is worse than none, which is how the first version of this shipped.
    """
    import subprocess

    try:
        import yaml
    except ImportError:
        return ["stale check skipped: pyyaml not available"]

    lock = ROOT / "catalog.lock.yaml"
    if not lock.is_file():
        return ["stale check skipped: no catalog.lock.yaml"]
    data = yaml.safe_load(lock.read_text())
    entry = (data.get("apps") or data).get(app)
    if not entry:
        return [f"stale check skipped: {app} absent from catalog.lock.yaml"]
    versions = [v for v in (entry.get("versions") or []) if v.get("digest")]
    if not versions:
        return [f"stale check skipped: {app} has no published digest"]
    # EVERY published version, not the newest. A VEX covers the image, and an app
    # ships several lines at once: picking one would call a statement stale that
    # is live in another and invite pruning a real suppression. "Newest by
    # publish date" is doubly wrong here, since a patch to an older line can
    # publish after a newer line (kgateway 2.3.7 postdates 2.4.3).
    found: set[str] = set()
    scanned = []
    for v in versions:
        ref = f"{entry['image']}@{v['digest']}"
        try:
            out = subprocess.run(
                ["trivy", "image", "--quiet", "--format", "json", "--scanners", "vuln",
                 "--detection-priority", "comprehensive",
                 "--severity", "CRITICAL,HIGH,MEDIUM,LOW,UNKNOWN", ref],
                capture_output=True, text=True, timeout=900, check=True).stdout
            report = json.loads(out)
        except Exception as e:  # noqa: BLE001 - report, never swallow
            return [f"stale check FAILED on {v['version']}: {str(e)[:110]}"]
        found |= {x.get("VulnerabilityID")
                  for r in (report.get("Results") or [])
                  for x in (r.get("Vulnerabilities") or [])}
        scanned.append(str(v["version"]))

    claimed = [st.get("vulnerability", {}).get("name")
               for st in (json.loads(path.read_text()).get("statements") or [])]
    return [f"stale, in none of {', '.join(scanned)}: {c}"
            for c in claimed if c and c not in found]


def main() -> int:
    only = [a for a in sys.argv[1:] if not a.startswith('-')]
    files = sorted((ROOT / "apps").glob("*/vex.openvex.json"))
    if only:
        files = [f for f in files if f.parent.name in only]
        missing = set(only) - {f.parent.name for f in files}
        for m in sorted(missing):
            print(f"!! {m}: no apps/{m}/vex.openvex.json")
        if missing:
            return 3
    if not files:
        print("no VEX files found; nothing to check")
        return 0

    # Staleness needs a real scan, so it is opt-in rather than part of the
    # default well-formedness check.
    want_stale = "--stale" in sys.argv
    only = [a for a in only if not a.startswith("-")]

    bad = 0
    for f in files:
        app = f.parent.name
        errs = check_file(f) + check_workflow_wiring(app)
        if want_stale:
            errs += check_stale(app, f)
        if errs:
            bad += 1
            print(f"!! {app}")
            for e in errs:
                print(f"     {e}")
        else:
            n = len(json.loads(f.read_text()).get("statements") or [])
            print(f"ok {app}: {n} statement(s)")
    if bad:
        print(f"\n{bad} VEX file(s) rejected. A VEX entry removes a finding from the "
              f"0-CVE gate, so it has to be airtight.")
    return 1 if bad else 0


if __name__ == "__main__":
    raise SystemExit(main())
