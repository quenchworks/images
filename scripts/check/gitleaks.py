#!/usr/bin/env python3
# gitleaks: GitHub releases of gitleaks/gitleaks (stable vX.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("gitleaks", [v for v in github("gitleaks/gitleaks") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
