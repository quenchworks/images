#!/usr/bin/env python3
# k6: GitHub releases (grafana/k6), tags vX.Y.Z. NEWEST-ONLY. Upstream still
# patches the v1 line alongside v2; vkey sorting keeps v2 on top, which is the
# line the recipe ships.
from _lib import github, report
report("k6", github("grafana/k6"), n=1)
