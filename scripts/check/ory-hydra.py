#!/usr/bin/env python3
# ory-hydra: GitHub releases of ory/hydra (stable vX.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("ory-hydra", [v for v in github("ory/hydra") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
