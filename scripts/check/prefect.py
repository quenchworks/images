#!/usr/bin/env python3
# prefect: PyPI releases of prefect (stable X.Y.Z only). Newest-only, like the recipe.
import re
from _lib import pypi, report
report("prefect", [v for v in pypi("prefect") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
