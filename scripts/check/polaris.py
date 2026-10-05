#!/usr/bin/env python3
# polaris: GitHub releases; tags drop the v before 10.2 (10.1.8) and carry it after (v10.2.5).
# Window = latest patch of last 3 minor lines.
from _lib import github, report_lines
report_lines("polaris", github("FairwindsOps/polaris"), 2)
