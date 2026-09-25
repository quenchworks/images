#!/usr/bin/env python3
# hubble-relay: GitHub releases of cilium/cilium (stable X.Y.Z), newest patch per minor line.
import re
from _lib import github, report_lines
report_lines("hubble-relay", [v for v in github("cilium/cilium") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
