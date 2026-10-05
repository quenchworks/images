#!/usr/bin/env python3
# versitygw: GitHub releases, tag vX.Y.Z. Window = latest patch of last 3 minor lines.
from _lib import github, report_lines
report_lines("versitygw", github("versity/versitygw"), 2)
