#!/usr/bin/env python3
# trufflehog: GitHub releases of trufflesecurity/trufflehog (stable X.Y.Z); newest patch per
# minor line, like the recipe's three lines.
import re
from _lib import github, report_lines
report_lines("trufflehog", [v for v in github("trufflesecurity/trufflehog") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
