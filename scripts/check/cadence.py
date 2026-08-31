#!/usr/bin/env python3
# cadence: GitHub releases (uber/cadence), tags vX.Y.Z. NEWEST-ONLY per
# build.conf, so a single flat candidate.
from _lib import github, report
report("cadence", github("uber/cadence"), n=1)
