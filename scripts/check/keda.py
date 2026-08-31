#!/usr/bin/env python3
# keda: GitHub releases (kedacore/keda), tags vX.Y.Z. NEWEST-ONLY per build.conf.
from _lib import github, report
report("keda", github("kedacore/keda"), n=1)
