#!/usr/bin/env python3
# flyway: GitHub releases of flyway/flyway (tags flyway-X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
vs = [re.sub(r"^flyway-", "", v) for v in github("flyway/flyway")]
report("flyway", [v for v in vs if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
