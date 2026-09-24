#!/usr/bin/env python3
# metallb-speaker: GitHub releases of metallb/metallb, vX.Y.Z tags only (the repo also tags
# metallb-chart-X.Y.Z releases). Newest-only, like the recipe.
import re
from _lib import github, report
report("metallb-speaker", [t for t in github("metallb/metallb") if re.fullmatch(r"\d+\.\d+\.\d+", t)], n=1)
