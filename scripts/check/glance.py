#!/usr/bin/env python3
# glance: GitHub releases on glanceapp/glance, tags vX.Y.Z. LATEST-ONLY.
from _lib import github, report
report("glance", github("glanceapp/glance"), n=1)
