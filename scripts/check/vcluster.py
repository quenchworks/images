#!/usr/bin/env python3
# vcluster: GitHub releases, tag vX.Y.Z. Window = latest patch of last 3 minor lines.
from _lib import github, report
report("vcluster", github("loft-sh/vcluster"))
