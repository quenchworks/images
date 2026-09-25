#!/usr/bin/env python3
# akhq: GitHub releases of tchiotludo/akhq (stable X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("akhq", [v for v in github("tchiotludo/akhq") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
