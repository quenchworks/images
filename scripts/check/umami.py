#!/usr/bin/env python3
# umami: GitHub releases on umami-software/umami, tags vX.Y.Z. LATEST-ONLY.
from _lib import github, report
report("umami", github("umami-software/umami"), n=1)
