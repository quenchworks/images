#!/usr/bin/env python3
# skywalking: build fetches archive.apache.org/dist/skywalking/<ver>/ (the OAP
# binary dist). Upstream cuts one GitHub Release per version; SkyWalking ships a
# single active major (10.x) with one patch per minor line, so we track the
# newest patch of each minor line (depth=2), mirroring the recipe's latest-3.
from _lib import github, report_lines
report_lines("skywalking", github("apache/skywalking"), 2)
