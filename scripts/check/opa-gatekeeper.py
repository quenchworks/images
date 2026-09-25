#!/usr/bin/env python3
# opa-gatekeeper: GitHub releases of open-policy-agent/gatekeeper (stable X.Y.Z), newest patch per minor line.
import re
from _lib import github, report_lines
report_lines("opa-gatekeeper", [v for v in github("open-policy-agent/gatekeeper") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
