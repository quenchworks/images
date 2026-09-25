#!/usr/bin/env python3
# kubo: GitHub releases, tag vX.Y.Z. Per minor line (0.41, 0.42, 0.43), newest patch of each.
from _lib import github, report_lines
report_lines("kubo", github("ipfs/kubo"), 2)
