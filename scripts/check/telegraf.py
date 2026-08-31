#!/usr/bin/env python3
# telegraf: GitHub releases (influxdata/telegraf), tags vX.Y.Z. build.conf ships
# the latest patch of the last 3 minor lines (1.37, 1.38, 1.39), so check per
# line at depth 2.
from _lib import github, report_lines
report_lines("telegraf", github("influxdata/telegraf"), 2)
