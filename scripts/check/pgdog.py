#!/usr/bin/env python3
# pgdog: GitHub releases, tag vX.Y.Z. Latest only, like the recipe.
from _lib import github, report
report("pgdog", github("pgdogdev/pgdog"), n=1)
