#!/usr/bin/env python3
# openziti: GitHub releases of openziti/ziti (vX.Y.Z). Two lines (2.0 and 1.6), like the recipe.
import re
from _lib import github, report_lines
report_lines("openziti", [v for v in github("openziti/ziti") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
