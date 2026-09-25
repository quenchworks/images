#!/usr/bin/env python3
# checkov: PyPI releases of checkov (stable X.Y.Z). Newest-only, like the recipe.
import re
from _lib import pypi, report
report("checkov", [v for v in pypi("checkov") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
