#!/usr/bin/env python3
# label-studio: PyPI releases, final X.Y.Z only. Latest line only: 1.20 to 1.22
# carry CVE-2026-22033, fixed in 1.23.
import re
from _lib import pypi, report
report("label-studio", [v for v in pypi("label-studio") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
