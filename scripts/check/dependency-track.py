#!/usr/bin/env python3
# dependency-track: GitHub releases (DependencyTrack/dependency-track), tag X.Y.Z. We ship
# the 5.1 line (see apps/dependency-track/build.conf); a newer line shows as NEW LINE.
from _lib import github, report_lines
report_lines("dependency-track", github("DependencyTrack/dependency-track"), 2)
