#!/usr/bin/env python3
# yourls: GitHub releases of YOURLS/YOURLS, tags without a v. One active line.
from _lib import github, report
report("yourls", github("YOURLS/YOURLS"), n=1)
