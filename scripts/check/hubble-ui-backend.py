#!/usr/bin/env python3
# hubble-ui-backend: GitHub releases of cilium/hubble-ui (stable X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("hubble-ui-backend", [v for v in github("cilium/hubble-ui") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
