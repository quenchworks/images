#!/usr/bin/env python3
# apache-pinot: GitHub releases of apache/pinot (tags release-X.Y.Z), newest-only.
import re
from _lib import github, report
report("apache-pinot", [m[1] for t in github("apache/pinot") if (m := re.fullmatch(r"(?:release-)?(\d+\.\d+\.\d+)", t))], n=1)
