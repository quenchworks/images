#!/usr/bin/env python3
# cluster-autoscaler: GitHub releases of kubernetes/autoscaler, tag cluster-autoscaler-X.Y.Z
# (the repo also tags charts as cluster-autoscaler-chart-*). One line per Kubernetes minor.
import re
from _lib import github, report_lines
report_lines("cluster-autoscaler", github("kubernetes/autoscaler"), 2,
             keep=lambda t: re.match(r"cluster-autoscaler-\d", t),
             clean=lambda t: t.removeprefix("cluster-autoscaler-"))
