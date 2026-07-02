#!/usr/bin/env python3
# xyops: GitHub releases (pixlcore/xyops), tag vX.Y.Z, prereleases dropped by
# github(). Single 1.0.x line -> window is the newest 3 patch releases (report's
# default n = len(VERSIONS) = 3).
from _lib import github, report
report("xyops", github("pixlcore/xyops"))
