#!/usr/bin/env python3
# cloudnative-pg: GitHub releases, tag vX.Y.Z. Latest patch of the last 3 minor lines.
from _lib import github, report
report("cloudnative-pg", github("cloudnative-pg/cloudnative-pg"))
