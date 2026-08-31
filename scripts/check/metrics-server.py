#!/usr/bin/env python3
# metrics-server: GitHub releases (kubernetes-sigs/metrics-server). Same
# two-streams-one-namespace trap as descheduler: the binary is vX.Y.Z while the
# repo`s own Helm chart is metrics-server-helm-chart-3.X.Y. Verified on the
# releases API -- the chart tags are NUMERICALLY HIGHER (3.14.0 vs 0.9.0), so
# leaving them in would make the checker think we are permanently behind by a
# chart version. Drop them. NEWEST-ONLY per build.conf.
from _lib import github, report
report("metrics-server", github("kubernetes-sigs/metrics-server"), n=1,
       keep=lambda t: not t.startswith("metrics-server-helm-chart"))
