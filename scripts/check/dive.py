#!/usr/bin/env python3
# dive: GitHub releases (wagoodman/dive). Newest line only, per build.conf.
from _lib import github, report
report("dive", github("wagoodman/dive"), n=1)
