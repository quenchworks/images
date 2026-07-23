#!/usr/bin/env python3
# tigerbeetle: single fast-moving release line, GitHub releases tagged X.Y.Z.
from _lib import github, report
report("tigerbeetle", github("tigerbeetle/tigerbeetle"), n=3)
