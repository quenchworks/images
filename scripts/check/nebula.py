#!/usr/bin/env python3
# nebula: GitHub releases on slackhq/nebula, tags vX.Y.Z. Newest patch of the last three minor lines.
from _lib import github, report_lines
report_lines("nebula", github("slackhq/nebula"), 2)
