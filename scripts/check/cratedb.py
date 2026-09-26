#!/usr/bin/env python3
# cratedb: GitHub releases (crate/crate), tag X.Y.Z. We ship the 6.4 line (see
# apps/cratedb/build.conf); a newer line shows as NEW LINE.
from _lib import github, report_lines
report_lines("cratedb", github("crate/crate"), 2)
