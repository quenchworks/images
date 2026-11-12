#!/usr/bin/env python3
# smokescreen: GitHub releases on stripe/smokescreen, tags vX.Y.Z. LATEST-ONLY.
from _lib import github, report
report("smokescreen", github("stripe/smokescreen"), n=1)
