#!/usr/bin/env python3
# dagster: PyPI releases of dagster (stable X.Y.Z only). Newest-only, like the recipe.
import re
from _lib import pypi, report
report("dagster", [v for v in pypi("dagster") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
