#!/usr/bin/env python3
# authentik: GitHub releases (goauthentik/authentik). CalVer tags are prefixed
# `version/` (e.g. `version/2026.5.3`); strip that to the bare YYYY.M.P. We ship the
# newest patch of each of the last 3 stable MINOR lines, so compare per-line at
# depth=2 (the YYYY.M minor line), matching the recipe's VERSIONS.
from _lib import github, report_lines


def clean(tag):
    # `github()` lstrips a leading `v`, so `version/2026.5.3` arrives as
    # `ersion/2026.5.3`. Take whatever follows the last `/` (the bare YYYY.M.P).
    return tag.rsplit("/", 1)[-1]


report_lines(
    "authentik",
    github("goauthentik/authentik"),
    depth=2,
    keep=lambda t: "/" in t,
    clean=clean,
)
