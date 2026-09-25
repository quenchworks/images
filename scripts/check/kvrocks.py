#!/usr/bin/env python3
# kvrocks: GitHub releases of apache/kvrocks (stable vX.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("kvrocks", [v for v in github("apache/kvrocks") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
