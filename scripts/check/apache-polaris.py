#!/usr/bin/env python3
# apache-polaris: GitHub releases of apache/polaris (tags apache-polaris-X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("apache-polaris", [v.removeprefix("apache-polaris-") for v in github("apache/polaris") if re.fullmatch(r"apache-polaris-\d+\.\d+\.\d+", v)], n=1)
