#!/usr/bin/env python3
# ansible: PyPI `ansible` (the community meta-package). We ship the newest 1-2 stable
# community releases, so compare the newest n=2 to what we have. Drop prereleases
# (rc/b/a/dev) — keep pure X.Y.Z numeric versions.
from _lib import pypi, report
report("ansible", pypi("ansible"), n=2, keep=lambda v: v.replace(".", "").isdigit())
