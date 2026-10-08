#!/usr/bin/env python3
# linkerd-viz-metrics-api: built from the linkerd/linkerd2 edge-YY.M.P tags linkerd-control-plane ships;
# window = newest patch of the last 3 edge year.month lines, like linkerd-control-plane.
from _lib import github, report_lines
tags = [t[len("edge-"):] for t in github("linkerd/linkerd2") if t.startswith("edge-")]
report_lines("linkerd-viz-metrics-api", tags, 2)
