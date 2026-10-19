#!/usr/bin/env python3
# camel-k: GitHub releases of apache/camel-k (stable X.Y.Z), newest patch per minor line.
import re
from _lib import github, report_lines
report_lines("camel-k", [v for v in github("apache/camel-k") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
