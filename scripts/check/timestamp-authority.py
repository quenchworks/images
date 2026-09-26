#!/usr/bin/env python3
# timestamp-authority: GitHub releases of sigstore/timestamp-authority, newest
# patch of each line we ship (2.1, 2.0, 1.2).
from _lib import github, report_lines
report_lines("timestamp-authority", github("sigstore/timestamp-authority"), depth=2)
