#!/usr/bin/env python3
# nessie: GitHub releases of projectnessie/nessie (tags nessie-X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("nessie", [v.removeprefix("nessie-") for v in github("projectnessie/nessie") if re.fullmatch(r"nessie-\d+\.\d+\.\d+", v)], n=1)
