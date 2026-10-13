#!/usr/bin/env python3
# grist: GitHub releases of gristlabs/grist-core (stable X.Y.Z), newest-only.
import re
from _lib import github, report
report("grist", [v for v in github("gristlabs/grist-core") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
