#!/usr/bin/env python3
# superset: PyPI (apache-superset). Ship the newest patch of each of the last 3 stable
# MINOR lines (6.1 / 6.0 / 5.0 today), so compare per-line at depth=2 (the X.Y minor
# line), matching the recipe's VERSIONS. Drop rc/dev/bN prereleases — keep pure X.Y.Z.
from _lib import pypi, report_lines

report_lines(
    "superset",
    pypi("apache-superset"),
    depth=2,
    keep=lambda v: v.replace(".", "").isdigit(),
)
