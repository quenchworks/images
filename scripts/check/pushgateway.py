#!/usr/bin/env python3
# pushgateway: GitHub releases on prometheus/pushgateway, tags vX.Y.Z. LATEST-ONLY.
from _lib import github, report
report("pushgateway", github("prometheus/pushgateway"), n=1)
