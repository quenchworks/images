#!/usr/bin/env python3
# notation: GitHub releases of notaryproject/notation (stable vX.Y.Z), newest-only.
import re
from _lib import github, report
report("notation", [v for v in github("notaryproject/notation") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
