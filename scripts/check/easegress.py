#!/usr/bin/env python3
# easegress: GitHub releases (easegress-io/easegress), tag vX.Y.Z. Per minor line
# (2.9, 2.10, 2.11), newest patch of each.
from _lib import github, report_lines
report_lines("easegress", github("easegress-io/easegress"), 2)
