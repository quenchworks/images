#!/usr/bin/env python3
# freshrss: GitHub releases on FreshRSS/FreshRSS, tags X.Y.Z (no v). LATEST-ONLY.
from _lib import github, report
report("freshrss", github("FreshRSS/FreshRSS"), n=1)
