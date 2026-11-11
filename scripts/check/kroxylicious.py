#!/usr/bin/env python3
# kroxylicious: GitHub releases of kroxylicious/kroxylicious (vX.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("kroxylicious", [v for v in github("kroxylicious/kroxylicious") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
