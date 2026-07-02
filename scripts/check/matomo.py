#!/usr/bin/env python3
# matomo: matomo-org/matomo GitHub releases (stable, non-prerelease). We ship the
# newest patch of the last 3 MINOR lines (5.9, 5.10, 5.11); tag = X.Y.Z, no `v`.
from _lib import github, report_lines
report_lines("matomo", github("matomo-org/matomo"), depth=2)
