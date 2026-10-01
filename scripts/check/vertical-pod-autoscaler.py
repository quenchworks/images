#!/usr/bin/env python3
# vertical-pod-autoscaler: kubernetes/autoscaler releases tagged vertical-pod-autoscaler-X.Y.Z
# (chart tags vertical-pod-autoscaler-chart-* excluded), newest patch per minor line.
# github() strips a leading "v" from every tag, so the prefix arrives as "ertical-...".
from _lib import github, report_lines
p = "ertical-pod-autoscaler-"
report_lines("vertical-pod-autoscaler",
             [t[len(p):] for t in github("kubernetes/autoscaler") if t.startswith(p) and not t.startswith(p + "chart")], 2)
