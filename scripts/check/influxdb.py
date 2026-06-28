#!/usr/bin/env python3
# influxdb: GitHub releases (influxdata/influxdb). POLICY: we hold the 2.x line —
# 3.x is a different product (full rewrite, Core/Enterprise split), not a bump.
# Filter to 2.x so the check reports staleness WITHIN 2.x, not the 3.x jump.
from _lib import github, report
report("influxdb", github("influxdata/influxdb"), keep=lambda v: v.startswith("2."))
