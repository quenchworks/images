#!/usr/bin/env python3
# trickster: GitHub releases on trickstercache/trickster, tags vX.Y.Z. Newest patch of the last three minor lines.
from _lib import github, report_lines
report_lines("trickster", github("trickstercache/trickster"), 2)
