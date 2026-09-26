#!/usr/bin/env python3
# jupyterhub: PyPI releases, final X.Y.Z only (betas filtered). Latest patch of
# the last 3 minor lines.
import re
from _lib import pypi, report
report("jupyterhub", [v for v in pypi("jupyterhub") if re.fullmatch(r"\d+\.\d+\.\d+", v)])
