#!/usr/bin/env python3
# argo-rollouts: GitHub releases of argoproj/argo-rollouts (stable X.Y.Z), newest patch per minor line.
import re
from _lib import github, report_lines
report_lines("argo-rollouts", [v for v in github("argoproj/argo-rollouts") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
