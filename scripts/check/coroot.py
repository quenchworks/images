#!/usr/bin/env python3
# coroot: GitHub releases (coroot/coroot), tags vX.Y.Z. NEWEST-ONLY per build.conf.
from _lib import github, report
report("coroot", github("coroot/coroot"), n=1)
