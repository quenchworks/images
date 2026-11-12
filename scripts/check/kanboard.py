#!/usr/bin/env python3
# kanboard: GitHub releases on kanboard/kanboard, tags vX.Y.Z. LATEST-ONLY.
from _lib import github, report
report("kanboard", github("kanboard/kanboard"), n=1)
