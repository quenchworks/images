# VEX policy: when a finding may be suppressed

`apps/<app>/vex.openvex.json` is picked up automatically by `scripts/build-image.sh`
and passed to Trivy as `--vex`. **A statement in that file removes a finding from the
0-CVE gate.** That gate is the product guarantee, so this file is not documentation —
it is production suppression logic, and it is the one place where a careless entry can
hide a real vulnerability from the check designed to catch it.

Every file is validated by `scripts/check-vex.py` before Trivy is allowed to use it.
An invalid file fails the build. Run it over the whole catalog any time:

```sh
uv run scripts/check-vex.py            # every app that has a VEX file
uv run scripts/check-vex.py jenkins-inbound-agent
```

## The bar

A statement is legitimate only when the vulnerable code **cannot** be reached in the
image we ship, and you have checked that rather than reasoned about it. In practice
that means one of:

- **The vulnerable component is not in the image at all.** Usually a Wolfi secdb
  advisory keyed to a shared package *origin*, inherited by every subpackage even
  though only one of them ships the vulnerable library. Prove it by extracting every
  layer and searching for the artifact.
- **The vulnerable code path is not present** in the subset we ship (a library
  compiled without the affected feature, a jar with the vulnerable classes stripped).
- **The finding names a version that is not what actually ships**, because the
  scanner read stale metadata rather than the artifact. Fix the metadata if you can;
  VEX is the fallback when you cannot.

## Never acceptable

- **A CVE that is real but inconvenient.** If the vulnerable code ships, the answer is
  a fix, a version bump, a jar swap, or an honest `BLOCKED=1` — never VEX.
- **Metadata surgery that quiets the scanner while the code stays vulnerable.** This
  was prototyped for opensearch's relocated httpcore5 classes and rejected for exactly
  this reason: Trivy went quiet while the running bytecode was untouched. Making a
  scanner agree with you is not the same as being right.
- **`status: under_investigation`.** A hunch must never suppress a finding. The
  validator rejects any status other than `not_affected`.
- **An `impact_statement` that asserts rather than demonstrates.** "False positive" is
  a conclusion. Write down what you ran and what it showed; the validator enforces a
  length floor and looks for evidence language, but a reviewer still has to agree.

## Shape of a statement

```json
{
  "vulnerability": { "name": "CVE-2026-12345" },
  "products": [ { "@id": "pkg:apk/wolfi/some-subpackage" } ],
  "status": "not_affected",
  "justification": "vulnerable_code_not_present",
  "impact_statement": "PROVEN false-positive. <what the advisory claims> <why it does not apply here> Verified by <the exact check> which returned <the result>."
}
```

`justification` must be one of the five OpenVEX values:
`component_not_present`, `vulnerable_code_not_present`,
`vulnerable_code_not_in_execute_path`,
`vulnerable_code_cannot_be_controlled_by_adversary`,
`inline_mitigations_already_exist`.

Scope products as narrowly as the evidence supports. A statement covering four
subpackages needs the claim to hold for all four.

## Maintenance

VEX entries go stale. When an upstream fix lands, or a recipe stops shipping the
component, the statement stops being needed — and a stale statement is a liability,
because it may suppress a future finding that IS real. Bump the document `version`
and the date in `@id` on every edit, and prune entries whose CVE no longer appears in
a scan of the current image.

`scripts/check-vex.py` can prove a file is well-formed and argued. It cannot prove a
claim is true. That part is review.
