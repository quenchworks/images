#!/usr/bin/env python3
# unleash: npm releases of unleash-server (stable X.Y.Z), newest patch of the last 3 minor lines.
import re
from _lib import npm, report_lines
report_lines("unleash", [v for v in npm("unleash-server") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
