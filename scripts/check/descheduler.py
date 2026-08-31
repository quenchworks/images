#!/usr/bin/env python3
# descheduler: GitHub releases (kubernetes-sigs/descheduler). The repo cuts TWO
# release streams from one tag namespace -- the binary (vX.Y.Z) and its own Helm
# chart (descheduler-helm-chart-X.Y.Z). Verified against the releases API: both
# appear interleaved, and the chart tags share the same numbers, so leaving them
# in would compare our image version against a chart version. Drop them.
# NEWEST-ONLY per build.conf.
from _lib import github, report
report("descheduler", github("kubernetes-sigs/descheduler"), n=1,
       keep=lambda t: not t.startswith("descheduler-helm-chart"))
