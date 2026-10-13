#!/usr/bin/env python3
# satosa: PyPI releases of SATOSA (stable X.Y.Z), newest patch of the last 3 minors.
import re
from _lib import pypi, report_lines
report_lines("satosa", [v for v in pypi("satosa") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
