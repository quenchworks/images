#!/usr/bin/env python3
# jupyterhub: PyPI releases, final X.Y.Z only (betas filtered). Latest patch of
# the last 2 minor lines (5.4 is dropped: 5.4.6 carries CVE-2026-54338).
import re
from _lib import pypi, report
report("jupyterhub", [v for v in pypi("jupyterhub") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=2)
