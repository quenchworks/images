#!/usr/bin/env python3
# terralist: GitHub releases of terralist/terralist (stable X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("terralist", [v for v in github("terralist/terralist") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
