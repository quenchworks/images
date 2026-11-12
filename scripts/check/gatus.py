#!/usr/bin/env python3
# gatus: GitHub releases on TwiN/gatus, tags vX.Y.Z. Newest patch of the last three minor lines.
from _lib import github, report_lines
report_lines("gatus", github("TwiN/gatus"), 2)
