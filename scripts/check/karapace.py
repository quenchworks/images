#!/usr/bin/env python3
# karapace: GitHub releases of Aiven-Open/karapace (stable X.Y.Z), newest-only.
import re
from _lib import github, report
report("karapace", [v for v in github("Aiven-Open/karapace") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
