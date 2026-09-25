#!/usr/bin/env python3
# oras: GitHub releases of oras-project/oras (stable vX.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("oras", [v for v in github("oras-project/oras") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
