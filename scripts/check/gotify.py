#!/usr/bin/env python3
# gotify: GitHub releases on gotify/SERVER (not gotify/gotify -- the server is
# its own repo and is what the recipe builds), tags vX.Y.Z. NEWEST-ONLY.
from _lib import github, report
report("gotify", github("gotify/server"), n=1)
