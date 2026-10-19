#!/usr/bin/env python3
# netdata: GitHub releases (netdata/netdata), stable vX.Y.Z only (nightly tags carry a suffix).
import re
from _lib import github, report
report("netdata", [v for v in github("netdata/netdata") if re.fullmatch(r"\d+\.\d+\.\d+", v)])
