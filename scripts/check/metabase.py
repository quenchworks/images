#!/usr/bin/env python3
# metabase: GitHub releases of metabase/metabase. The open-source edition is
# tagged v0.X.Y (v1.X.Y is Enterprise), so only v0 tags count. The window compares
# the newest release; report()'s same-line pass covers the older line the chart
# pins (0.63 as of 2026-10-20), so a 0.63.x patch next to have=0.64.x is a real
# update for the chart's line, not noise.
from _lib import github, report
report("metabase", [t for t in github("metabase/metabase") if t.startswith("0.")], n=1)
