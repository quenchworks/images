#!/usr/bin/env python3
# reloader: GitHub releases (stakater/Reloader); chart-v* tags are filtered out by github().
# Upstream maintains one line, so latest only.
from _lib import github, report
report("reloader", [v for v in github("stakater/Reloader") if not v.startswith("chart")], n=1)
