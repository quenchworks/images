#!/usr/bin/env python3
# mealie: GitHub releases on mealie-recipes/mealie, tags vX.Y.Z. LATEST-ONLY.
from _lib import github, report
report("mealie", github("mealie-recipes/mealie"), n=1)
