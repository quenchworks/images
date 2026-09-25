#!/usr/bin/env python3
# pgadmin: PyPI releases of pgadmin4 (stable X.Y), newest-only like the recipe.
import re
from _lib import pypi, report
report("pgadmin", [v for v in pypi("pgadmin4") if re.fullmatch(r"\d+\.\d+", v)], n=1)
