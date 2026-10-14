#!/usr/bin/env python3
# plone: PyPI releases of the Plone distribution (stable X.Y.Z), newest only.
import re
from _lib import pypi, report
report("plone", [v for v in pypi("Plone") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
