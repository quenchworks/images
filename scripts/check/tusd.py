#!/usr/bin/env python3
# tusd: GitHub releases on tus/tusd, tags vX.Y.Z. LATEST-ONLY.
from _lib import github, report
report("tusd", github("tus/tusd"), n=1)
