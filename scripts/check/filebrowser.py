#!/usr/bin/env python3
# filebrowser: GitHub releases (filebrowser/filebrowser), tags vX.Y.Z. Upstream
# is on a single fast-moving 2.63.x patch line; build.conf ships the newest two,
# so the default n=len(VERSIONS) window is right.
from _lib import github, report
report("filebrowser", github("filebrowser/filebrowser"))
