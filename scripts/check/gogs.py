#!/usr/bin/env python3
# gogs: GitHub releases of gogs/gogs (stable X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("gogs", [v for v in github("gogs/gogs") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
