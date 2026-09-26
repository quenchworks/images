#!/usr/bin/env python3
# fulcio: GitHub releases of sigstore/fulcio, newest patch of each line we ship
# (1.8, 1.7). 1.6 is held; see apps/fulcio/build.conf.
from _lib import github, report_lines
report_lines("fulcio", github("sigstore/fulcio"), depth=2)
