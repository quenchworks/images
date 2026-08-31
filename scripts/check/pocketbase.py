#!/usr/bin/env python3
# pocketbase: GitHub releases (pocketbase/pocketbase), tags vX.Y.Z. NEWEST-ONLY.
# Upstream still backports to an old 0.22.x line in parallel with 0.40.x; vkey
# sorting keeps the current line on top, which is the one the recipe ships.
from _lib import github, report
report("pocketbase", github("pocketbase/pocketbase"), n=1)
