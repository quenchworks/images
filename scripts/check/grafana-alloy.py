#!/usr/bin/env python3
# grafana-alloy: GitHub releases (grafana/alloy), tags vX.Y.Z. build.conf ships
# the latest patch of the last 3 minor lines, so check per line at depth 2.
# Upstream occasionally leaves an "untagged-<sha>" release in the list; report_lines
# drops any candidate that does not start with a digit, so it is filtered already.
from _lib import github, report_lines
report_lines("grafana-alloy", github("grafana/alloy"), 2)
