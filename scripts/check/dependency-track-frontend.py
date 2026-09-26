#!/usr/bin/env python3
# dependency-track-frontend: GitHub releases (DependencyTrack/frontend), tag X.Y.Z. Tracks
# the API server line (see apps/dependency-track/build.conf).
from _lib import github, report_lines
report_lines("dependency-track-frontend", github("DependencyTrack/frontend"), 2)
