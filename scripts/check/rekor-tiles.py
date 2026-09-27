#!/usr/bin/env python3
# rekor-tiles: GitHub releases of sigstore/rekor-tiles, newest patch of each
# line we ship (2.3, 2.2, 2.1).
from _lib import github, report_lines
report_lines("rekor-tiles", github("sigstore/rekor-tiles"), depth=2)
