#!/usr/bin/env python3
# argo-events: GitHub releases of argoproj/argo-events (stable X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("argo-events", [v for v in github("argoproj/argo-events") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
