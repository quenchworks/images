#!/usr/bin/env python3
# owncast: GitHub releases on owncast/owncast, tags vX.Y.Z. LATEST-ONLY.
from _lib import github, report
report("owncast", github("owncast/owncast"), n=1)
