#!/usr/bin/env python3
# litellm: PyPI releases (stable X.Y.Z only). Newest-only, like the recipe.
import re
from _lib import pypi, report
report("litellm", [v for v in pypi("litellm") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
