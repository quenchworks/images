#!/usr/bin/env python3
# skopeo: GitHub releases of containers/skopeo (stable vX.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("skopeo", [v for v in github("containers/skopeo") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
