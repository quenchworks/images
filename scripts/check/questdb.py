#!/usr/bin/env python3
# questdb: GitHub releases, tag X.Y.Z (no v prefix). We ship the latest 3 stable.
from _lib import github, report
report("questdb", github("questdb/questdb"), n=3)
