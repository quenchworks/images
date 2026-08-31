#!/usr/bin/env python3
# pyroscope: GitHub releases (grafana/pyroscope), tags vX.Y.Z. NEWEST-ONLY.
# Upstream publishes releases out of chronological order (a v1.21.x patch lands
# after v2.x), so rely on vkey sorting rather than list order.
from _lib import github, report
report("pyroscope", github("grafana/pyroscope"), n=1)
